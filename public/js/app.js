/**
 * Main Application Entry Point
 * Orchestrates all modules and initializes the Whale Radar application
 */

import { CONFIG } from './config.js';
import { debounce, safeGetElement, setupTableInfiniteScroll, formatNumber, renderWhaleAlignmentBadge } from './utils.js';
import * as API from './api.js';
import { renderWhaleAlertsTable, renderRunningPositions, renderSummaryTable, updateStatsTicker, renderStockCorrelations, renderProfitLossHistory, renderDailyPerformance, renderCandleTable, renderTechnicalAnalysis } from './render.js?v=12';
import { createWhaleAlertSSE } from './sse-handler.js';
import { initStrategySystem } from './strategy-manager.js';
import { initWebhookManagement } from './webhook-config.js';
import { initAIAnalysis, openAIAnalysisModal } from './ai-analysis.js';
import { initNotifications, requestNotificationPermission, playSound, showDesktopNotification, AppSettings } from './notifications.js';

// Configure marked.js for markdown rendering
if (typeof marked !== 'undefined') {
    marked.use({
        breaks: true,
        gfm: true,
        headerIds: false,
        mangle: false
    });
}

// Application state
const state = {
    alerts: [],
    stats: {},
    currentOffset: 0,
    isLoading: false,
    hasMore: true,
    currentFilters: {
        search: '',
        action: 'ALL',
        amount: 0,
        board: 'ALL'
    },
    whaleSSE: null,
    patternSSE: null,
    currentPatternType: 'accumulation',
    // Optimization: Track visibility and active tabs
    isPageVisible: true,
    activeAnalyticsTab: 'correlations-view',
    pollingIntervalId: null,
    statsIntervalId: null,
    // Table-specific state for lazy loading
    tables: {
        history: {
            offset: 0,
            hasMore: true,
            isLoading: false,
            data: [],
            filters: {}
        },
        signals: {
            offset: 0,
            hasMore: true,
            isLoading: false,
            data: []
        },
        performance: {
            offset: 0,
            hasMore: true,
            isLoading: false,
            data: []
        },
        accumulation: {
            offset: 0,
            hasMore: true,
            isLoading: false,
            data: []
        },
        distribution: {
            offset: 0,
            hasMore: true,
            isLoading: false,
            data: []
        }
    }
};

// OPTIMIZATION: Visibility API - pause polling when tab is hidden
document.addEventListener('visibilitychange', () => {
    state.isPageVisible = !document.hidden;
    if (state.isPageVisible) {
        console.log('📊 Tab visible - resuming polling');
    } else {
        console.log('⏸️ Tab hidden - pausing non-essential polling');
    }
});

/**
 * Initialize the application
 */
async function init() {
    console.log('Initializing Whale Radar Application...');

    // Initial data load
    try {
        // Critical data - will not fully block on individual module errors
        await Promise.all([
            fetchAlerts(true),
            fetchStats().catch(err => console.warn('Initial stats fetch failed:', err)),
            API.fetchAccumulationSummary()
                .then(renderAccumulationSummary)
                .catch(err => {
                    console.error('Initial accumulation fetch failed:', err);
                    const timeEl = document.getElementById('bandar-timeframe');
                    if (timeEl) timeEl.innerHTML = '<span class="text-accentDanger">⚠️ Gagal memuat data</span>';

                    const accTbody = safeGetElement('accumulation-table-body');
                    if (accTbody) accTbody.innerHTML = '<tr><td colspan="4" class="text-center p-8 text-accentDanger"><span class="text-2xl block mb-2">⚠️</span>Gagal memuat data Akumulasi.</td></tr>';

                    const distTbody = safeGetElement('distribution-table-body');
                    if (distTbody) distTbody.innerHTML = '<tr><td colspan="4" class="text-center p-8 text-accentDanger"><span class="text-2xl block mb-2">⚠️</span>Gagal memuat data Distribusi.</td></tr>';
                }),
            API.fetchRunningPositions()
                .then(renderPositions)
                .catch(err => {
                    console.error('Initial positions fetch failed:', err);
                    renderPositions(null); // Passing null triggers error UI in renderPositions
                })
        ]);
    } catch (error) {
        console.error('Initial data load error:', error);
    }

    // Setup event listeners
    setupFilterControls();
    setupModals();
    setupAnalyticsTabs();
    setupAnalyticsTabs();
    setupInfiniteScroll();
    setupProfitLossHistory();
    setupAccumulationTables();

    // Initialize strategy system
    initStrategySystem();

    // Initialize webhook management
    initWebhookManagement();

    // Initialize AI analysis system
    initAIAnalysis();

    // Setup mobile filter toggle
    setupMobileFilterToggle();

    // Setup running trades toggle
    setupRunningTradesToggle();

    // Setup newly added feature controls
    setupFeatureControls();

    // Setup bandar heatmap tabs
    setupBandarTabs();

    // Connect SSE for real-time updates
    connectWhaleAlertSSE();

    // Start polling for analytics
    startAnalyticsPolling();

    // Initialize notification system
    initNotifications();

    console.log('Application initialized successfully');
}

/**
 * Fetch whale alerts
 * @param {boolean} reset - Reset pagination
 */
async function fetchAlerts(reset = false) {
    if (state.isLoading) return;
    if (!reset && !state.hasMore) return;

    state.isLoading = true;
    const loadingDiv = safeGetElement('loading');
    const loadingMore = safeGetElement('loading-more');
    const noMoreData = safeGetElement('no-more-data');

    console.log(`🔍 Fetching alerts... Reset: ${reset}, Offset: ${state.currentOffset}, HasMore: ${state.hasMore}`);

    if (reset) {
        if (loadingDiv) loadingDiv.style.display = 'block';
        if (noMoreData) noMoreData.style.display = 'none';
    } else {
        // Show "loading more" indicator at bottom
        if (loadingMore) loadingMore.style.display = 'flex';
        if (noMoreData) noMoreData.style.display = 'none';
    }

    const tbody = safeGetElement('alerts-table-body');

    try {
        const offset = reset ? 0 : state.currentOffset;
        const data = await API.fetchAlerts(state.currentFilters, offset);

        const alerts = data.data || [];
        state.hasMore = data.has_more || false;

        console.log(`✅ Received ${alerts.length} alerts. Total: ${state.alerts.length + alerts.length}, HasMore: ${state.hasMore}`);

        if (reset) {
            state.alerts = alerts;
            state.currentOffset = alerts.length;
            renderWhaleAlertsTable(state.alerts, tbody, loadingDiv, false);
        } else {
            state.alerts = state.alerts.concat(alerts);
            state.currentOffset += alerts.length;
            renderWhaleAlertsTable(alerts, tbody, loadingDiv, true);
        }

        // Show "no more data" if we've reached the end
        if (!state.hasMore && state.alerts.length > 0 && noMoreData) {
            noMoreData.style.display = 'block';
        }
    } catch (error) {
        console.error('Failed to fetch alerts:', error);
        if (tbody && reset) {
            tbody.innerHTML = '<tr><td colspan="7" class="text-center p-8 text-accentDanger"><span class="text-2xl block mb-2">⚠️</span>Gagal memuat data Whale Alerts. Silakan coba lagi.</td></tr>';
        }
        state.hasMore = false;
    } finally {
        state.isLoading = false;
        if (loadingDiv) loadingDiv.style.display = 'none';
        if (loadingMore) loadingMore.style.display = 'none';
    }
}

/**
 * Fetch global statistics
 */
async function fetchStats() {
    try {
        state.stats = await API.fetchStats();
        updateStatsTicker(state.stats);
    } catch (error) {
        console.error('Failed to fetch stats:', error);
        updateStatsTicker({ total_whale_trades: '-', win_rate: null, avg_profit_pct: null });
    }
}

/**
 * Refresh all data
 */
async function refreshAllData() {
    // Critical data
    await Promise.all([
        fetchAlerts(true),
        fetchStats().catch(err => console.warn('Stats fetch failed:', err))
    ]);

    // Optional analytics - don't block on errors
    API.fetchAnalyticsHub().catch(err => console.warn('Analytics hub unavailable'));

    // Reload correlations if tab is active
    if (document.getElementById('correlations-view')?.classList.contains('active')) {
        loadCorrelations();
    }
}

/**
 * Setup filter controls and event listeners
 */
function setupFilterControls() {
    // Refresh button
    const refreshBtn = safeGetElement('refresh-btn');
    if (refreshBtn) {
        refreshBtn.addEventListener('click', () => {
            updateFilters();
            state.currentOffset = 0;
            state.hasMore = true;
            refreshAllData();

            refreshBtn.style.transform = 'rotate(360deg)';
            setTimeout(() => refreshBtn.style.transform = '', 300);
        });
    }

    // Search with debouncing
    const searchInput = safeGetElement('search');
    if (searchInput) {
        searchInput.addEventListener('input', debounce((e) => {
            updateFilters();
            state.currentOffset = 0;
            state.hasMore = true;
            refreshAllData();
        }, CONFIG.SEARCH_DEBOUNCE_MS));
    }

    // Filter dropdowns
    ['filter-action', 'filter-amount', 'filter-board'].forEach(id => {
        const element = safeGetElement(id);
        if (element) {
            element.addEventListener('change', () => {
                updateFilters();
                state.currentOffset = 0;
                state.hasMore = true;
                refreshAllData();
                highlightActiveFilters();
            });
        }
    });

    // Clear filters button
    const clearBtn = safeGetElement('clear-filters-btn');
    if (clearBtn) {
        clearBtn.addEventListener('click', () => {
            document.getElementById('search').value = '';
            document.getElementById('filter-action').value = 'ALL';
            document.getElementById('filter-amount').value = '0';
            document.getElementById('filter-board').value = 'ALL';

            updateFilters();
            state.currentOffset = 0;
            state.hasMore = true;
            refreshAllData();
        });
    }
}

/**
 * Setup mobile filter toggle
 */
function setupMobileFilterToggle() {
    const toggleBtn = safeGetElement('mobile-filter-toggle');
    const filterContent = safeGetElement('filter-content');
    const toggleIcon = safeGetElement('filter-toggle-icon');

    if (toggleBtn && filterContent && toggleIcon) {
        toggleBtn.addEventListener('click', () => {
            filterContent.classList.toggle('show');
            toggleIcon.textContent = filterContent.classList.contains('show') ? '▲' : '▼';
        });
    }
}

/**
 * Setup running trades toggle functionality
 */
function setupBandarTabs() {
    const tabs = document.querySelectorAll('.bandar-tab');
    tabs.forEach(tab => {
        tab.addEventListener('click', () => {
            tabs.forEach(t => {
                t.classList.remove('active', 'text-accentInfo', 'border-b-2', 'border-accentInfo');
                t.classList.add('text-textMuted');
            });
            tab.classList.add('active', 'text-accentInfo', 'border-b-2', 'border-accentInfo');
            tab.classList.remove('text-textMuted');

            const target = tab.dataset.target;
            const tablesView = document.getElementById('bandar-tables-view');
            const heatmapView = document.getElementById('bandar-heatmap-view');

            if (target === 'bandar-tables-view') {
                tablesView.classList.remove('hidden');
                heatmapView.classList.add('hidden');
            } else {
                tablesView.classList.add('hidden');
                heatmapView.classList.remove('hidden');
                renderBandarTreemap();
            }
        });
    });
}

function renderBandarTreemap() {
    const canvas = document.getElementById('bandar-treemap');
    if (!canvas) return;

    // Combine data
    const allData = [
        ...state.tables.accumulation.data.map(d => ({ ...d, type: 'acc', value: d.total_value })),
        ...state.tables.distribution.data.map(d => ({ ...d, type: 'dist', value: d.total_value }))
    ];

    if (allData.length === 0) return;

    if (window.bandarChart) {
        window.bandarChart.destroy();
    }

    const ctx = canvas.getContext('2d');
    window.bandarChart = new Chart(ctx, {
        type: 'treemap',
        data: {
            datasets: [{
                label: 'Aktivitas Bandar',
                tree: allData,
                key: 'value',
                groups: ['type', 'stock_symbol'],
                backgroundColor: (ctx) => {
                    if (ctx.type !== 'data') return 'transparent';
                    const data = ctx.raw._data;
                    // Opacity based on magnitude of dominance
                    const isAcc = data.type === 'acc';
                    const pct = isAcc ? data.buy_percentage : data.sell_percentage;
                    const opacity = Math.min(Math.max((pct - 50) / 50, 0.3), 1.0);

                    return isAcc ? `rgba(14, 203, 129, ${opacity})` : `rgba(246, 70, 93, ${opacity})`;
                },
                borderColor: '#2f3339',
                borderWidth: 1,
                spacing: 1,
                labels: {
                    align: 'center',
                    display: true,
                    color: '#fff',
                    font: { size: 11, weight: 'bold' },
                    formatter: (ctx) => {
                        if (ctx.type !== 'data') return;
                        const d = ctx.raw._data;
                        const pct = d.type === 'acc' ? d.buy_percentage : d.sell_percentage;
                        return [d.stock_symbol, `${pct.toFixed(0)}%`];
                    }
                }
            }]
        },
        options: {
            maintainAspectRatio: false,
            plugins: {
                tooltip: {
                    callbacks: {
                        title: (items) => items[0].raw._data.stock_symbol,
                        label: (item) => {
                            const d = item.raw._data;
                            const formatVal = (val) => {
                                if (val >= 1_000_000_000) return `Rp ${(val / 1_000_000_000).toFixed(1)}M`;
                                return `Rp ${(val / 1_000_000).toFixed(1)}Jt`;
                            };
                            return d.type === 'acc'
                                ? `Akumulasi ${d.buy_percentage.toFixed(1)}% | Val: ${formatVal(d.total_value)}`
                                : `Distribusi ${d.sell_percentage.toFixed(1)}% | Val: ${formatVal(d.total_value)}`;
                        }
                    }
                },
                legend: { display: false }
            }
        }
    });
}

function setupFeatureControls() {
    // Notification Toggle
    const notifBtn = document.getElementById('notification-toggle-btn');
    if (notifBtn) {
        // Init icon based on current state
        notifBtn.className = AppSettings.soundEnabled ?
            'p-2 rounded-lg border border-borderColor hover:bg-bgHover transition-colors text-lg text-accentSuccess' :
            'p-2 rounded-lg border border-borderColor hover:bg-bgHover transition-colors text-lg text-textMuted opacity-50';

        notifBtn.innerHTML = AppSettings.soundEnabled ? '🔔' : '🔕';

        notifBtn.addEventListener('click', () => {
            AppSettings.soundEnabled = !AppSettings.soundEnabled;
            AppSettings.desktopNotifications = AppSettings.soundEnabled; // tie them together for simplicity

            if (AppSettings.soundEnabled) {
                requestNotificationPermission();
                notifBtn.className = 'p-2 rounded-lg border border-borderColor hover:bg-bgHover transition-colors text-lg text-accentSuccess';
                notifBtn.innerHTML = '🔔';
                showToast('Notifications Enabled', 'success');
            } else {
                notifBtn.className = 'p-2 rounded-lg border border-borderColor hover:bg-bgHover transition-colors text-lg text-textMuted opacity-50';
                notifBtn.innerHTML = '🔕';
                showToast('Notifications Disabled', 'info');
            }
        });
    }

    // Mock Trading Panel Toggle
    const mockToggleBtn = document.getElementById('mock-trade-toggle-btn');
    const mockPanel = document.getElementById('mock-control-panel');
    if (mockToggleBtn && mockPanel) {
        mockToggleBtn.addEventListener('click', (e) => {
            e.stopPropagation();
            mockPanel.classList.toggle('hidden');
        });

        // Close when clicking outside
        document.addEventListener('click', (e) => {
            if (!mockPanel.contains(e.target) && !mockToggleBtn.contains(e.target)) {
                mockPanel.classList.add('hidden');
            }
        });
    }

    // Mock Aggressiveness Slider
    const aggSlider = document.getElementById('mock-aggressiveness');
    const aggVal = document.getElementById('mock-aggressiveness-val');
    if (aggSlider && aggVal) {
        aggSlider.addEventListener('input', (e) => {
            const val = e.target.value;
            if (val === '1') { aggVal.textContent = 'Safe'; aggVal.className = 'text-xs font-bold text-accentSuccess'; }
            else if (val === '2') { aggVal.textContent = 'Medium'; aggVal.className = 'text-xs font-bold text-accentInfo'; }
            else { aggVal.textContent = 'High Risk'; aggVal.className = 'text-xs font-bold text-accentDanger'; }
        });
    }
}

/**
 * Setup running trades toggle functionality
 */
function setupRunningTradesToggle() {
    const toggleBtn = document.getElementById('toggle-running-trades');
    const container = document.getElementById('running-trades-container');
    const arrow = document.getElementById('running-trades-arrow');
    const statusEl = document.getElementById('trade-stream-status');

    if (toggleBtn && container && arrow) {
        // Load saved preference
        const savedState = localStorage.getItem('runningTradesVisible');
        if (savedState === 'false') {
            state.runningTradesVisible = false;
            container.style.height = '0px';
            container.style.opacity = '0';
            container.style.marginTop = '0';
            arrow.style.transform = 'rotate(-90deg)';
            if (statusEl) {
                statusEl.textContent = '⏸️ Paused (Hidden)';
                statusEl.className = 'text-[10px] font-mono text-textMuted'; // Remove pulse
            }
        }

        toggleBtn.addEventListener('click', () => {
            state.runningTradesVisible = !state.runningTradesVisible;
            localStorage.setItem('runningTradesVisible', state.runningTradesVisible);

            if (state.runningTradesVisible) {
                // Show
                container.style.height = '300px';
                container.style.opacity = '1';
                container.style.marginTop = '0.75rem'; // mt-3 is 0.75rem
                arrow.style.transform = 'rotate(0deg)';
                if (statusEl) {
                    statusEl.textContent = '⚫ Live';
                    statusEl.className = 'text-[10px] font-mono text-accentSuccess animate-pulse';
                }
            } else {
                // Hide
                container.style.height = '0px';
                container.style.opacity = '0';
                container.style.marginTop = '0';
                arrow.style.transform = 'rotate(-90deg)';
                if (statusEl) {
                    statusEl.textContent = '⏸️ Paused (Hidden)';
                    statusEl.className = 'text-[10px] font-mono text-textMuted';
                }
            }
        });
    }
}

/**
 * Update filter state from DOM
 */
function updateFilters() {
    state.currentFilters = {
        search: document.getElementById('search')?.value || '',
        action: document.getElementById('filter-action')?.value || 'ALL',
        amount: parseInt(document.getElementById('filter-amount')?.value || '0'),
        board: document.getElementById('filter-board')?.value || 'ALL'
    };

    // Show/hide clear button
    const hasFilters = state.currentFilters.search ||
        state.currentFilters.action !== 'ALL' ||
        state.currentFilters.amount > 0 ||
        state.currentFilters.board !== 'ALL';

    const clearBtn = safeGetElement('clear-filters-btn');
    if (clearBtn) {
        clearBtn.style.display = hasFilters ? 'block' : 'none';
    }
}

/**
 * Highlight active filters
 */
function highlightActiveFilters() {
    ['filter-action', 'filter-amount', 'filter-board'].forEach(id => {
        const element = document.getElementById(id);
        if (element) {
            if (element.value !== 'ALL' && element.value !== '0') {
                element.classList.add('border-accentInfo', 'ring-1', 'ring-accentInfo');
                element.classList.remove('border-borderColor');
            } else {
                element.classList.remove('border-accentInfo', 'ring-1', 'ring-accentInfo');
                element.classList.add('border-borderColor');
            }
        }
    });
}



/**
 * Setup infinite scroll for whale alerts table
 */
function setupInfiniteScroll() {
    setupTableInfiniteScroll({
        tableBodyId: 'alerts-table-body',
        fetchFunction: () => fetchAlerts(false),
        getHasMore: () => state.hasMore,
        getIsLoading: () => state.isLoading,
        noMoreDataId: 'no-more-data'
    });
}

/**
 * Connect to market SSE (Whales + Running Trades)
 */
function connectWhaleAlertSSE() {
    state.whaleSSE = createWhaleAlertSSE({
        onAlert: (newAlert) => {
            // BLOCK SSE updates if filters are active
            const hasActiveFilters = (
                state.currentFilters.search !== '' ||
                state.currentFilters.action !== 'ALL' ||
                state.currentFilters.amount > 0 ||
                state.currentFilters.board !== 'ALL'
            );

            if (hasActiveFilters) {
                // console.log('⏸️ SSE update blocked - filters active');
                return;
            }

            // Play sound and show notification if enabled
            if (AppSettings.soundEnabled) {
                playSound(newAlert.action === 'BUY' ? 'BUY' : (newAlert.action === 'SELL' ? 'SELL' : 'WHALE'));
            }
            if (AppSettings.desktopNotifications) {
                const act = newAlert.action === 'BUY' ? '🟢 HAKA' : '🔴 HAKI';
                showDesktopNotification(
                    `Whale Alert: ${newAlert.stock_symbol} ${act}`,
                    `Rp ${(newAlert.trigger_value / 1000000).toFixed(1)}M di harga ${newAlert.trigger_price}`
                );
            }

            // Prepend new alert
            state.alerts.unshift(newAlert);
            if (state.alerts.length > CONFIG.MAX_ALERTS_CACHE) {
                state.alerts.pop();
            }
            state.currentOffset = Math.min(state.currentOffset + 1, CONFIG.MAX_ALERTS_CACHE);

            const tbody = safeGetElement('alerts-table-body');
            const loadingDiv = safeGetElement('loading');
            renderWhaleAlertsTable(state.alerts, tbody, loadingDiv);

            // Refresh stats
            fetchStats();
        },
        onTrade: (trade) => {
            // Update connection status
            const statusEl = document.getElementById('trade-stream-status');
            if (statusEl) {
                statusEl.textContent = '⚫ Live';
                statusEl.className = 'text-[10px] font-mono text-accentSuccess animate-pulse';
            }

            // Render trade row
            renderRunningTrade(trade);
        },
        onError: (error) => {
            console.error('SSE Error:', error);
            const statusEl = document.getElementById('trade-stream-status');
            if (statusEl) {
                statusEl.textContent = '🔴 Disconnected';
                statusEl.className = 'text-[10px] font-mono text-accentDanger';
            }
        }
    });
}

/**
 * Render a single running trade row
 * @param {Object} trade 
 */
function renderRunningTrade(trade) {
    // OPTIMIZATION: Skip rendering if section is hidden
    if (state.runningTradesVisible === false) return;

    const tbody = document.getElementById('running-trades-body');
    if (!tbody) return;

    // Check if user is scrolling (pause updates if scrolled down)
    const container = tbody.closest('.table-wrapper');
    if (container && container.scrollTop > 5) {
        // Update status to indicate paused due to scrolling
        const statusEl = document.getElementById('trade-stream-status');
        if (statusEl && !statusEl.textContent.includes('Scrolling')) {
            statusEl.textContent = '⏸️ Paused (Scrolling)';
            statusEl.className = 'text-[10px] font-mono text-accentWarning';
        }
        return;
    } else {
        // Reset status if it was paused
        const statusEl = document.getElementById('trade-stream-status');
        if (statusEl && statusEl.textContent.includes('Scrolling')) {
            statusEl.textContent = '⚫ Live';
            statusEl.className = 'text-[10px] font-mono text-accentSuccess animate-pulse';
        }
    }

    const row = document.createElement('tr');
    row.className = 'hover:bg-bgHover transition-colors border-b border-borderColor last:border-0';

    // Format Time
    const timeDate = new Date(trade.time);
    const timeStr = timeDate.toLocaleTimeString('id-ID', { hour: '2-digit', minute: '2-digit', second: '2-digit' });

    // Determine Color
    const actionClass = trade.action === 'BUY' ? 'text-accentSuccess' : (trade.action === 'SELL' ? 'text-accentDanger' : 'text-textSecondary');
    const priceClass = trade.change_pct > 0 ? 'text-accentSuccess' : (trade.change_pct < 0 ? 'text-accentDanger' : 'text-textPrimary');

    // Format Value abbreviator
    const formatValue = (val) => {
        if (val >= 1_000_000_000) return (val / 1_000_000_000).toFixed(1) + 'M';
        if (val >= 1_000_000) return (val / 1_000_000).toFixed(1) + 'Jt';
        return (val / 1_000).toFixed(0) + 'Rb';
    };

    row.innerHTML = `
        <td class="px-4 py-1.5 text-textMuted">${timeStr}</td>
        <td class="px-4 py-1.5 font-bold text-textPrimary">${trade.symbol}</td>
        <td class="px-4 py-1.5 text-right font-bold ${priceClass}">${trade.price}</td>
        <td class="px-4 py-1.5 text-right text-textSecondary">${trade.volume_lot.toLocaleString('id-ID')}</td>
        <td class="px-4 py-1.5 text-right text-textPrimary">${formatValue(trade.value)}</td>
        <td class="px-4 py-1.5 text-center font-bold ${actionClass}">${trade.action === 'BUY' ? 'B' : (trade.action === 'SELL' ? 'S' : 'U')}</td>
    `;

    // Prepend and limit rows
    tbody.insertBefore(row, tbody.firstChild);

    // Keep max 20 rows to prevent DOM bloat
    if (tbody.children.length > 20) {
        tbody.removeChild(tbody.lastChild);
    }
}

/**
 * Render accumulation summary
 * @param {Object} data - Summary data
 * @param {boolean} reset - Reset pagination
 */
function renderAccumulationSummary(data, reset = true) {
    const accumulation = data.accumulation || [];
    const distribution = data.distribution || [];

    // Update timeframe display
    const timeframeEl = document.getElementById('bandar-timeframe');
    if (timeframeEl && data.timeframe) {
        timeframeEl.textContent = data.timeframe;
        // Add market status indicator
        if (data.market_status) {
            const statusColors = {
                'PRE_MARKET': 'text-yellow-500',
                'SESSION_1': 'text-green-500',
                'LUNCH_BREAK': 'text-orange-500',
                'SESSION_2': 'text-green-500',
                'PRE_CLOSING': 'text-yellow-500',
                'POST_MARKET': 'text-gray-500',
                'CLOSED': 'text-gray-500'
            };
            const statusClass = statusColors[data.market_status] || 'text-textSecondary';
            timeframeEl.innerHTML = `${data.timeframe} <span class="${statusClass}">(${data.market_status})</span>`;
        }
    }

    // Update state
    if (reset) {
        state.tables.accumulation.data = accumulation;
        state.tables.accumulation.offset = accumulation.length;
        state.tables.accumulation.hasMore = data.accumulation_has_more !== undefined ? data.accumulation_has_more : accumulation.length >= 50;

        state.tables.distribution.data = distribution;
        state.tables.distribution.offset = distribution.length;
        state.tables.distribution.hasMore = data.distribution_has_more !== undefined ? data.distribution_has_more : distribution.length >= 50;
    } else {
        state.tables.accumulation.data = state.tables.accumulation.data.concat(accumulation);
        state.tables.accumulation.offset += accumulation.length;
        state.tables.accumulation.hasMore = data.accumulation_has_more !== undefined ? data.accumulation_has_more : accumulation.length >= 50;

        state.tables.distribution.data = state.tables.distribution.data.concat(distribution);
        state.tables.distribution.offset += distribution.length;
        state.tables.distribution.hasMore = data.distribution_has_more !== undefined ? data.distribution_has_more : distribution.length >= 50;
    }

    // Update counters
    const accCount = safeGetElement('accumulation-count');
    const distCount = safeGetElement('distribution-count');
    if (accCount) accCount.textContent = state.tables.accumulation.data.length;
    if (distCount) distCount.textContent = state.tables.distribution.data.length;

    // Render tables with accumulated data
    const accTbody = safeGetElement('accumulation-table-body');
    const accPlaceholder = safeGetElement('accumulation-placeholder');
    renderSummaryTable('accumulation', state.tables.accumulation.data, accTbody, accPlaceholder);

    const distTbody = safeGetElement('distribution-table-body');
    const distPlaceholder = safeGetElement('distribution-placeholder');
    renderSummaryTable('distribution', state.tables.distribution.data, distTbody, distPlaceholder);

    // If heatmap is active, update it too
    if (document.getElementById('bandar-heatmap-view') && !document.getElementById('bandar-heatmap-view').classList.contains('hidden')) {
        renderBandarTreemap();
    }
}

/**
 * Load more accumulation data
 */
async function loadMoreAccumulation() {
    if (state.tables.accumulation.isLoading || !state.tables.accumulation.hasMore) return;

    state.tables.accumulation.isLoading = true;

    try {
        const data = await API.fetchAccumulationSummary(50, state.tables.accumulation.offset);
        renderAccumulationSummary(data, false);
    } catch (error) {
        console.error('Failed to load more accumulation:', error);
        state.tables.accumulation.hasMore = false;
        const tbody = safeGetElement('accumulation-table-body');
        if (tbody && state.tables.accumulation.data.length === 0) {
            tbody.innerHTML = '<tr><td colspan="4" class="text-center p-8 text-accentDanger"><span class="text-2xl block mb-2">⚠️</span>Gagal memuat data Akumulasi. Silakan coba lagi.</td></tr>';
        }
    } finally {
        state.tables.accumulation.isLoading = false;
    }
}

/**
 * Load more distribution data
 */
async function loadMoreDistribution() {
    if (state.tables.distribution.isLoading || !state.tables.distribution.hasMore) return;

    state.tables.distribution.isLoading = true;

    try {
        const data = await API.fetchAccumulationSummary(50, state.tables.distribution.offset);
        renderAccumulationSummary(data, false);
    } catch (error) {
        console.error('Failed to load more distribution:', error);
        state.tables.distribution.hasMore = false;
        const tbody = safeGetElement('distribution-table-body');
        if (tbody && state.tables.distribution.data.length === 0) {
            tbody.innerHTML = '<tr><td colspan="4" class="text-center p-8 text-accentDanger"><span class="text-2xl block mb-2">⚠️</span>Gagal memuat data Distribusi. Silakan coba lagi.</td></tr>';
        }
    } finally {
        state.tables.distribution.isLoading = false;
    }
}

/**
 * Setup accumulation/distribution tables with infinite scroll
 */
function setupAccumulationTables() {
    // Setup infinite scroll for accumulation table
    setupTableInfiniteScroll({
        tableBodyId: 'accumulation-table-body',
        fetchFunction: () => loadMoreAccumulation(),
        getHasMore: () => state.tables.accumulation.hasMore,
        getIsLoading: () => state.tables.accumulation.isLoading
    });

    // Setup infinite scroll for distribution table
    setupTableInfiniteScroll({
        tableBodyId: 'distribution-table-body',
        fetchFunction: () => loadMoreDistribution(),
        getHasMore: () => state.tables.distribution.hasMore,
        getIsLoading: () => state.tables.distribution.isLoading
    });
}

/**
 * Render positions
 * @param {Object} data - Positions data
 */
function renderPositions(data) {
    const tbody = safeGetElement('positions-table-body');
    const placeholder = safeGetElement('positions-placeholder');

    if (!data) {
        if (tbody) tbody.innerHTML = '<tr><td colspan="6" class="text-center p-8 text-accentDanger"><span class="text-2xl block mb-2">⚠️</span>Gagal memuat posisi aktif. Silakan coba lagi.</td></tr>';
        if (placeholder) placeholder.style.display = 'none';
        return;
    }

    const positions = data.positions || [];
    renderRunningPositions(positions, tbody, placeholder);
}

/**
 * Setup modals
 */
function setupModals() {
    // Help modal
    setupHelpModal();

    // AI Analysis button
    setupAIAnalysisButton();

    // Candle modal
    setupCandleModal();

    // Followup modal
    setupFollowupModal();
}

/**
 * Setup help modal
 */
function setupHelpModal() {
    const helpBtn = safeGetElement('help-btn');
    const modal = safeGetElement('help-modal');
    const modalClose = safeGetElement('modal-close');
    const modalGotIt = safeGetElement('modal-got-it');

    if (!helpBtn || !modal) return;

    const closeModal = () => modal.classList.add('hidden');

    helpBtn.addEventListener('click', () => modal.classList.remove('hidden'));
    if (modalClose) modalClose.addEventListener('click', closeModal);
    if (modalGotIt) modalGotIt.addEventListener('click', closeModal);

    modal.addEventListener('click', (e) => {
        if (e.target === modal) closeModal();
    });

    document.addEventListener('keydown', (e) => {
        if (e.key === 'Escape' && !modal.classList.contains('hidden')) {
            closeModal();
        }
    });
}

/**
 * Setup AI Analysis button
 */
function setupAIAnalysisButton() {
    const aiBtn = safeGetElement('ai-analysis-btn');
    if (aiBtn) {
        aiBtn.addEventListener('click', () => {
            openAIAnalysisModal();
        });
    }
}

/**
 * Setup candle modal
 */
let candleChart = null;
let currentCandleSymbol = null;
let currentCandleTimeframe = '5m';

function setupCandleModal() {
    const modal = document.getElementById('candle-modal');
    const closeBtn = document.getElementById('candle-modal-close');
    const overlay = document.getElementById('candle-modal-overlay');

    if (!modal) return;

    const closeModal = () => modal.classList.add('hidden');

    if (closeBtn) closeBtn.addEventListener('click', closeModal);
    if (overlay) overlay.addEventListener('click', closeModal);

    // Timeframe tab switching
    document.querySelectorAll('.c-tab').forEach(tab => {
        tab.addEventListener('click', async () => {
            document.querySelectorAll('.c-tab').forEach(t => t.classList.remove('active', 'border-accentInfo', 'text-accentInfo'));
            tab.classList.add('active', 'border-accentInfo', 'text-accentInfo');
            currentCandleTimeframe = tab.dataset.timeframe;
            if (currentCandleSymbol) {
                await loadCandleData(currentCandleSymbol, currentCandleTimeframe);
            }
        });
    });

    // Make openCandleModal available globally
    window.openCandleModal = async (symbol) => {
        const title = document.getElementById('candle-modal-title');
        if (title) title.textContent = `📉 ${symbol} - Market Details`;
        
        // Reset to 5m timeframe
        currentCandleTimeframe = '5m';
        document.querySelectorAll('.c-tab').forEach(t => {
            t.classList.remove('active', 'border-accentInfo', 'text-accentInfo');
            if (t.dataset.timeframe === '5m') t.classList.add('active', 'border-accentInfo', 'text-accentInfo');
        });
        
        modal.classList.remove('hidden');
        currentCandleSymbol = symbol;
        await loadCandleData(symbol, currentCandleTimeframe);
    };
}

async function loadCandleData(symbol, timeframe) {
    const tbody = document.getElementById('candle-list-body');
    const chartCanvas = document.getElementById('candle-chart');
    const chartLoading = document.getElementById('candle-chart-loading');
    const analysisPanel = document.getElementById('analysis-panel');

    if (!tbody || !chartCanvas || !analysisPanel) return;

    // Show loading
    if (chartLoading) chartLoading.classList.remove('hidden');
    if (chartCanvas) chartCanvas.classList.add('hidden');
    tbody.innerHTML = '<tr><td colspan="6" class="text-center p-8 text-textSecondary">Loading...</td></tr>';
    analysisPanel.innerHTML = '<div class="text-center p-4 text-textSecondary">Loading analysis...</div>';

    try {
        const data = await API.fetchCandles(symbol, timeframe);
        
        // Hide loading, show chart
        if (chartLoading) chartLoading.classList.add('hidden');
        if (chartCanvas) chartCanvas.classList.remove('hidden');

        // Render candle table
        if (data && data.candles && data.candles.length > 0) {
            renderCandleTable(data.candles, tbody);
            renderCandleChart(data.candles, chartCanvas);
            renderTechnicalAnalysis(data.indicators || {}, analysisPanel);
        } else {
            tbody.innerHTML = '<tr><td colspan="6" class="text-center p-8 text-textSecondary">Data candle tidak tersedia untuk timeframe ini.</td></tr>';
            analysisPanel.innerHTML = '<div class="text-center p-8 text-textSecondary bg-bgSecondary rounded-lg border border-borderColor"><span class="text-2xl block mb-2 opacity-50">📉</span><p>Data analisis teknikal belum tersedia</p></div>';
        }
    } catch (error) {
        console.error('Failed to load candle data:', error);
        tbody.innerHTML = '<tr><td colspan="6" class="text-center p-8 text-accentDanger"><span class="text-2xl block mb-2">⚠️</span>Gagal mengambil data candle. Silakan coba lagi.</td></tr>';
        analysisPanel.innerHTML = '<div class="text-center p-8 text-accentDanger bg-bgSecondary rounded-lg border border-accentDanger/30"><span class="text-2xl block mb-2">⚠️</span><p>Gagal mengambil data analisis teknikal</p></div>';
        if (chartLoading) chartLoading.classList.add('hidden');
        if (chartCanvas) chartCanvas.classList.remove('hidden');
    }
}

function renderCandleChart(candles, canvas) {
    if (!candles || candles.length === 0 || !canvas) return;

    // Sort chronologically for chart
    const sortedCandles = [...candles].sort((a, b) => new Date(a.time) - new Date(b.time)).slice(-50);

    const labels = sortedCandles.map(c => {
        const date = new Date(c.time);
        return currentCandleTimeframe === '1d' 
            ? date.toLocaleDateString('id-ID', { month: 'short', day: 'numeric' })
            : date.toLocaleTimeString('id-ID', { hour: '2-digit', minute: '2-digit' });
    });

    const chartData = sortedCandles.map(c => ({
        x: c.time,
        o: c.open,
        h: c.high,
        l: c.low,
        c: c.close
    }));

    const volumes = sortedCandles.map(c => c.volume);
    const colors = sortedCandles.map(c => c.close >= c.open ? 'rgba(14, 203, 129, 0.5)' : 'rgba(246, 70, 93, 0.5)');

    // Destroy existing chart
    if (candleChart) {
        candleChart.destroy();
    }

    // Calculate SMA
    const sma20 = calculateSMA(sortedCandles.map(c => c.close), 20);
    const sma50 = calculateSMA(sortedCandles.map(c => c.close), 50);

    candleChart = new Chart(canvas, {
        type: 'line',
        data: {
            labels: labels,
            datasets: [
                {
                    label: 'Price',
                    data: sortedCandles.map(c => c.close),
                    borderColor: '#3b82f6',
                    backgroundColor: 'transparent',
                    yAxisID: 'y',
                    tension: 0.1,
                    pointRadius: 1,
                    pointHoverRadius: 4
                },
                {
                    label: 'SMA 20',
                    data: sma20,
                    borderColor: '#ffa500',
                    backgroundColor: 'transparent',
                    yAxisID: 'y',
                    tension: 0.1,
                    pointRadius: 0,
                    borderWidth: 1
                },
                {
                    label: 'SMA 50',
                    data: sma50,
                    borderColor: '#8b5cf6',
                    backgroundColor: 'transparent',
                    yAxisID: 'y',
                    tension: 0.1,
                    pointRadius: 0,
                    borderWidth: 1
                },
                {
                    label: 'Volume',
                    data: volumes,
                    type: 'bar',
                    backgroundColor: colors,
                    yAxisID: 'y1',
                    barPercentage: 0.5
                }
            ]
        },
        options: {
            responsive: true,
            maintainAspectRatio: false,
            interaction: {
                mode: 'index',
                intersect: false
            },
            plugins: {
                legend: {
                    display: true,
                    position: 'top',
                    labels: {
                        color: '#8b949e',
                        boxWidth: 10,
                        padding: 10,
                        font: { size: 10 }
                    }
                },
                tooltip: {
                    backgroundColor: '#1a1d23',
                    titleColor: '#e7e9ea',
                    bodyColor: '#e7e9ea',
                    borderColor: '#2f3339',
                    borderWidth: 1
                }
            },
            scales: {
                x: {
                    grid: { color: '#2f3339', drawBorder: false },
                    ticks: { color: '#6e7681', maxTicksLimit: 10, font: { size: 9 } }
                },
                y: {
                    type: 'linear',
                    position: 'right',
                    grid: { color: '#2f3339', drawBorder: false },
                    ticks: { color: '#6e7681', font: { size: 10 } }
                },
                y1: {
                    type: 'linear',
                    position: 'left',
                    grid: { display: false },
                    ticks: { display: false }
                }
            }
        }
    });
}

function calculateSMA(data, period) {
    const result = [];
    for (let i = 0; i < data.length; i++) {
        if (i < period - 1) {
            result.push(null);
        } else {
            let sum = 0;
            for (let j = 0; j < period; j++) {
                sum += data[i - j];
            }
            result.push(sum / period);
        }
    }
    return result;
}

/**
 * Setup followup modal
 */
function setupFollowupModal() {
    const modal = document.getElementById('followup-modal');
    const closeBtn = document.getElementById('followup-close');
    const overlay = document.getElementById('followup-modal-overlay');

    if (!modal) return;

    const closeModal = () => modal.classList.add('hidden');

    if (closeBtn) closeBtn.addEventListener('click', closeModal);
    if (overlay) overlay.addEventListener('click', closeModal);

    // Make openFollowupModal available globally
    window.openFollowupModal = async (alertId, symbol, price) => {
        // Update header info
        const symbolEl = document.getElementById('followup-symbol');
        const priceEl = document.getElementById('followup-trigger-price');
        if (symbolEl) symbolEl.textContent = symbol;
        if (priceEl) priceEl.textContent = 'Rp ' + formatNumber(price);
        
        // Show loading
        const dataContainer = document.getElementById('followup-data');
        if (dataContainer) {
            dataContainer.innerHTML = `
                <div class="flex justify-center p-8">
                    <div class="animate-spin h-8 w-8 border-4 border-borderColor border-t-accentInfo rounded-full"></div>
                </div>
            `;
        }
        
        modal.classList.remove('hidden');
        
        // Load followup data
        if (alertId) {
            await loadFollowupData(alertId, symbol, price);
        }
    };
}

async function loadFollowupData(alertId, symbol, triggerPrice) {
    const dataContainer = document.getElementById('followup-data');
    if (!dataContainer) return;

    try {
        const data = await API.fetchWhaleFollowup(alertId);
        renderFollowupData(data, symbol, triggerPrice, dataContainer);
    } catch (error) {
        console.error('Failed to load followup data:', error);
        dataContainer.innerHTML = `
            <div class="text-center p-8 text-accentDanger">
                <span class="text-3xl block mb-2">⚠️</span>
                <p>Failed to load followup data</p>
                <p class="text-sm mt-1 text-textMuted">${error.message}</p>
            </div>
        `;
    }
}

function renderFollowupData(data, symbol, triggerPrice, container) {
    if (!data || !data.followup) {
        container.innerHTML = `
            <div class="text-center p-8 text-textSecondary">
                <span class="text-3xl block mb-2">📊</span>
                <p>No followup data available</p>
            </div>
        `;
        return;
    }

    const f = data.followup;
    const currentPrice = f.current_price || triggerPrice;
    const priceChange = triggerPrice > 0 ? ((currentPrice - triggerPrice) / triggerPrice * 100) : 0;
    const changeClass = priceChange >= 0 ? 'text-accentSuccess' : 'text-accentDanger';
    const changeSign = priceChange >= 0 ? '+' : '';

    // Time analysis
    const timeElapsed = f.minutes_elapsed || 0;
    const timeText = timeElapsed >= 60 
        ? `${Math.floor(timeElapsed/60)}h ${timeElapsed%60}m` 
        : `${timeElapsed}m`;

    // Performance metrics
    const winRate = f.total_signals > 0 
        ? ((f.winning_signals / f.total_signals) * 100).toFixed(1) 
        : 0;
    const avgProfit = f.avg_profit_pct || 0;
    const avgLoss = f.avg_loss_pct || 0;

    // Price history if available
    let priceHistoryHtml = '';
    if (f.price_history && f.price_history.length > 0) {
        const historyItems = f.price_history.slice(0, 5).map(ph => `
            <div class="flex justify-between items-center py-2 border-b border-borderColor last:border-0">
                <span class="text-xs text-textMuted">${ph.time}</span>
                <span class="font-mono text-sm text-textPrimary">${formatNumber(ph.price)}</span>
                <span class="text-xs ${ph.change >= 0 ? 'text-accentSuccess' : 'text-accentDanger'}">
                    ${ph.change >= 0 ? '+' : ''}${ph.change.toFixed(2)}%
                </span>
            </div>
        `).join('');
        
        priceHistoryHtml = `
            <div class="mt-4">
                <h4 class="text-sm font-bold text-textPrimary mb-2">📈 Price Movement</h4>
                <div class="bg-bgSecondary rounded-lg p-3 border border-borderColor">
                    ${historyItems}
                </div>
            </div>
        `;
    }

    container.innerHTML = `
        <div class="space-y-4">
            <!-- Price Movement -->
            <div class="grid grid-cols-2 gap-4">
                <div class="bg-bgSecondary rounded-lg p-4 border border-borderColor">
                    <div class="text-[10px] text-textMuted uppercase tracking-wider mb-1">Trigger Price</div>
                    <div class="text-xl font-bold text-textPrimary">Rp ${formatNumber(triggerPrice)}</div>
                </div>
                <div class="bg-bgSecondary rounded-lg p-4 border border-borderColor">
                    <div class="text-[10px] text-textMuted uppercase tracking-wider mb-1">Current Price</div>
                    <div class="text-xl font-bold ${changeClass}">Rp ${formatNumber(currentPrice)}</div>
                    <div class="text-sm ${changeClass}">${changeSign}${priceChange.toFixed(2)}%</div>
                </div>
            </div>

            <!-- Time & Performance -->
            <div class="grid grid-cols-2 gap-4">
                <div class="bg-bgSecondary rounded-lg p-4 border border-borderColor">
                    <div class="text-[10px] text-textMuted uppercase tracking-wider mb-1">Time Elapsed</div>
                    <div class="text-lg font-bold text-textPrimary">${timeText}</div>
                </div>
                <div class="bg-bgSecondary rounded-lg p-4 border border-borderColor">
                    <div class="text-[10px] text-textMuted uppercase tracking-wider mb-1">Signal History</div>
                    <div class="text-lg font-bold text-textPrimary">${f.total_signals || 0} signals</div>
                    <div class="text-sm text-accentSuccess">Win Rate: ${winRate}%</div>
                </div>
            </div>

            <!-- Profit/Loss Stats -->
            <div class="grid grid-cols-2 gap-4">
                <div class="bg-bgSecondary rounded-lg p-4 border border-borderColor">
                    <div class="text-[10px] text-textMuted uppercase tracking-wider mb-1">Avg Win</div>
                    <div class="text-lg font-bold text-accentSuccess">+${avgProfit.toFixed(2)}%</div>
                </div>
                <div class="bg-bgSecondary rounded-lg p-4 border border-borderColor">
                    <div class="text-[10px] text-textMuted uppercase tracking-wider mb-1">Avg Loss</div>
                    <div class="text-lg font-bold text-accentDanger">${avgLoss.toFixed(2)}%</div>
                </div>
            </div>

            <!-- Historical Performance -->
            ${f.historical_performance ? `
            <div class="bg-bgSecondary rounded-lg p-4 border border-borderColor">
                <div class="text-[10px] text-textMuted uppercase tracking-wider mb-2">Historical Performance</div>
                <div class="grid grid-cols-3 gap-2 text-center">
                    <div>
                        <div class="text-lg font-bold text-accentSuccess">${f.historical_performance.wins || 0}</div>
                        <div class="text-[10px] text-textMuted">Wins</div>
                    </div>
                    <div>
                        <div class="text-lg font-bold text-accentDanger">${f.historical_performance.losses || 0}</div>
                        <div class="text-[10px] text-textMuted">Losses</div>
                    </div>
                    <div>
                        <div class="text-lg font-bold text-textPrimary">${f.historical_performance.total || 0}</div>
                        <div class="text-[10px] text-textMuted">Total</div>
                    </div>
                </div>
            </div>
            ` : ''}

            ${priceHistoryHtml}
        </div>
    `;
}

/**
 * Render order flow data
 * @param {Object} data - Order flow data
 */
function renderOrderFlow(data) {
    // Stub function - backend endpoint not implemented yet
    // Order flow visualization would go here
    if (data && data.symbol) {
        console.log('Order flow data:', data);
    }
}

/**
 * Setup analytics tabs
 */
function setupAnalyticsTabs() {
    const tabs = document.querySelectorAll('.s-tab');
    tabs.forEach(tab => {
        tab.addEventListener('click', () => {
            tabs.forEach(t => t.classList.remove('active'));
            tab.classList.add('active');

            const target = tab.dataset.target;

            // OPTIMIZATION: Track active tab for selective polling
            state.activeAnalyticsTab = target;

            document.querySelectorAll('.tab-panel').forEach(panel => {
                panel.classList.remove('active');
            });
            const targetPanel = document.getElementById(target);
            if (targetPanel) targetPanel.classList.add('active');

            // Load data if needed (lazy loading)
            if (target === 'correlations-view') {
                loadCorrelations();
            } else if (target === 'performance-view') {
                loadPerformance(true);
                // Setup infinite scroll for performance table (only once)
                if (!targetPanel.dataset.scrollSetup) {
                    setupTableInfiniteScroll({
                        tableBodyId: 'daily-performance-body',
                        fetchFunction: () => loadPerformance(false),
                        getHasMore: () => state.tables.performance.hasMore,
                        getIsLoading: () => state.tables.performance.isLoading
                    });
                    targetPanel.dataset.scrollSetup = 'true';
                }
            } else if (target === 'optimization-view') {
                loadOptimizationData();
            }
        });
    });
}

/**
 * Load strategy optimization data (EV, thresholds, effectiveness)
 */
async function loadOptimizationData() {
    console.log('📊 Loading strategy optimization data...');

    const evList = safeGetElement('ev-list');
    const thresholdList = safeGetElement('threshold-list');
    const effectivenessBody = safeGetElement('effectiveness-body');

    // Show loading states
    if (evList) evList.innerHTML = '<div class="p-4 text-center text-textMuted text-xs">Memuat data...</div>';
    if (thresholdList) thresholdList.innerHTML = '<div class="p-4 text-center text-textMuted text-xs">Memuat data...</div>';
    if (effectivenessBody) effectivenessBody.innerHTML = '<tr><td colspan="6" class="text-center p-4 text-textSecondary">Memuat data...</td></tr>';

    try {
        // Fetch all optimization data in parallel
        const [evData, thresholdData, effectivenessData] = await Promise.all([
            API.fetchExpectedValues(30).catch(() => ({ expected_values: [] })),
            API.fetchOptimalThresholds(30).catch(() => ({ thresholds: [] })),
            API.fetchStrategyEffectiveness(30).catch(() => ({ effectiveness: [] }))
        ]);

        // Render Expected Values
        if (evList) {
            const evs = evData.expected_values || [];
            if (evs.length === 0) {
                evList.innerHTML = '<div class="p-4 text-center text-textMuted text-xs">Belum ada data historis</div>';
            } else {
                evList.innerHTML = evs.map(ev => {
                    const evClass = ev.expected_value > 0 ? 'text-accentSuccess' : (ev.expected_value < 0 ? 'text-accentDanger' : '');
                    const recClass = ev.recommendation === 'STRONG' ? 'text-accentSuccess font-bold' :
                        (ev.recommendation === 'AVOID' ? 'text-accentDanger font-bold' : 'text-textSecondary');
                    return `
                        <div class="flex justify-between items-center py-2 border-b border-borderColor last:border-0">
                            <span class="font-semibold text-textPrimary text-sm">${ev.strategy}</span>
                            <div class="text-right">
                                <span class="${evClass} font-semibold text-sm mr-2">${ev.expected_value > 0 ? '+' : ''}${ev.expected_value.toFixed(4)}</span>
                                <span class="${recClass} text-xs px-1.5 py-0.5 bg-bgSecondary rounded">${ev.recommendation}</span>
                            </div>
                        </div>
                    `;
                }).join('');
            }
        }

        // Render Optimal Thresholds
        if (thresholdList) {
            const thresholds = thresholdData.thresholds || [];
            if (thresholds.length === 0) {
                thresholdList.innerHTML = '<div class="p-4 text-center text-textMuted text-xs">Belum ada data historis</div>';
            } else {
                thresholdList.innerHTML = thresholds.map(t => `
                    <div class="flex justify-between items-center py-2 border-b border-borderColor last:border-0">
                        <span class="font-semibold text-textPrimary text-sm">${t.strategy}</span>
                        <div class="text-right">
                            <span class="text-accentWarning font-semibold text-sm">${(t.recommended_min_conf * 100).toFixed(0)}%</span>
                            <span class="text-xs text-textSecondary ml-2">(${t.sample_size} sinyal)</span>
                        </div>
                    </div>
                `).join('');
            }
        }

        // Render Effectiveness Table
        if (effectivenessBody) {
            const effs = effectivenessData.effectiveness || [];
            if (effs.length === 0) {
                effectivenessBody.innerHTML = '<tr><td colspan="5" class="text-center p-4 text-textSecondary">Belum ada data historis</td></tr>';
            } else {
                effectivenessBody.innerHTML = effs.map(e => {
                    const wrClass = e.win_rate >= 50 ? 'text-accentSuccess' : (e.win_rate < 40 ? 'text-accentDanger' : 'text-textSecondary');
                    const evClass = e.expected_value > 0 ? 'text-accentSuccess' : (e.expected_value < 0 ? 'text-accentDanger' : '');
                    return `
                        <tr class="hover:bg-bgHover transition-colors border-b border-borderColor last:border-0">
                            <td class="p-3"><strong>${e.strategy}</strong></td>
                            <td class="p-3 text-right">${e.total_signals}</td>
                            <td class="p-3 text-right ${wrClass}">${e.win_rate.toFixed(1)}%</td>
                            <td class="p-3 text-right text-textPrimary">${e.avg_profit_pct.toFixed(2)}%</td>
                            <td class="p-3 text-right ${evClass}">${e.expected_value > 0 ? '+' : ''}${e.expected_value.toFixed(4)}</td>
                        </tr>
                    `;
                }).join('');
            }
        }

        console.log('✅ Strategy optimization data loaded successfully');
    } catch (error) {
        console.error('Failed to load optimization data:', error);
        if (evList) evList.innerHTML = '<div class="p-4 text-center text-accentDanger text-xs">Gagal memuat data</div>';
        if (thresholdList) thresholdList.innerHTML = '<div class="p-4 text-center text-accentDanger text-xs">Gagal memuat data</div>';
        if (effectivenessBody) effectivenessBody.innerHTML = '<tr><td colspan="6" class="text-center p-4 text-accentDanger">Gagal memuat data</td></tr>';
    }
}

// Export for global access (button onclick)
window.loadOptimizationData = loadOptimizationData;




/**
 * Start analytics polling with visibility optimization
 */
function startAnalyticsPolling() {
    const pollAnalytics = () => {
        // OPTIMIZATION: Skip polling if tab is hidden (except for critical data)
        if (!state.isPageVisible) {
            console.log('⏸️ Skipping analytics poll - tab hidden');
            return;
        }

        // Get symbol from search filter for baseline, default to IHSG
        const symbol = state.currentFilters.search || 'IHSG';

        // For Order Flow, use global data (empty symbol) if purely Dashboard view (IHSG)
        // This ensures the "Buy/Sell Pressure" bar shows aggregate market activity instead of 0
        const flowSymbol = symbol === 'IHSG' ? '' : symbol;

        // OPTIMIZATION: Only fetch data relevant to active view
        const promises = [
            // Always fetch these (critical for main dashboard)
            API.fetchOrderFlow(flowSymbol).then(renderOrderFlow).catch(() => null),
            API.fetchRunningPositions().then(renderPositions).catch(() => null)
        ];

        // Fetch tab-specific data only if that tab is active
        if (state.activeAnalyticsTab === 'performance-view') {
            promises.push(
                API.fetchDailyPerformance().then(data => {
                    renderDailyPerformance(data.performance || []);
                }).catch(() => null)
            );
        }

        Promise.all(promises).catch(error => {
            console.error('Analytics polling error:', error);
        });
    };

    // Initial fetch
    pollAnalytics();

    // Start polling with stored interval ID for potential cleanup
    state.pollingIntervalId = setInterval(pollAnalytics, CONFIG.ANALYTICS_POLL_INTERVAL);

    // Stats polling - only when visible
    state.statsIntervalId = setInterval(() => {
        if (state.isPageVisible) {
            fetchStats();
        }
    }, CONFIG.STATS_POLL_INTERVAL);
}


/**
 * Load and render correlations
 */
async function loadCorrelations() {
    const container = safeGetElement('correlation-container');
    const searchSymbol = state.currentFilters.search.toUpperCase();

    if (container) {
        container.innerHTML = '<div class="stream-loading">Memuat data korelasi...</div>';
    }

    try {
        // If searchSymbol is empty, this fetches global top correlations
        const data = await API.fetchCorrelations(searchSymbol);

        if (container) {
            if (!data.correlations || data.correlations.length === 0) {
                container.innerHTML = `
                    <div class="placeholder-small">
                        <span class="placeholder-icon">🔗</span>
                        <p>Belum ada data korelasi yang cukup.</p>
                    </div>
                `;
            } else {
                // Add header to indicate context
                const title = searchSymbol ? `Korelasi untuk ${searchSymbol}` : "Korelasi Terkuat (Global)";
                const header = `<div class="mb-2 font-semibold text-textSecondary text-sm">${title}</div>`;
                container.innerHTML = header;

                // Create a div for the list to append to
                const listDiv = document.createElement('div');
                renderStockCorrelations(data.correlations, listDiv);
                container.appendChild(listDiv);
            }
        }
    } catch (error) {
        console.error('Failed to load correlations:', error);
        if (container) {
            container.innerHTML = `
                <div class="flex flex-col items-center justify-center p-8 h-full text-textSecondary">
                    <p class="text-accentDanger">Gagal memuat data korelasi</p>
                </div>
            `;
        }
    }
}

/**
 * Setup profit/loss history section
 */
function setupProfitLossHistory() {
    const refreshBtn = safeGetElement('history-refresh-btn');
    const strategySelect = safeGetElement('history-strategy');
    const statusSelect = safeGetElement('history-status');
    const limitSelect = safeGetElement('history-limit');

    if (refreshBtn) {
        refreshBtn.addEventListener('click', () => {
            loadProfitLossHistory(true);
            refreshBtn.style.transform = 'rotate(360deg)';
            setTimeout(() => refreshBtn.style.transform = '', 300);
        });
    }

    // Auto-load on filter change
    if (strategySelect) {
        strategySelect.addEventListener('change', () => loadProfitLossHistory(true));
    }
    if (statusSelect) {
        statusSelect.addEventListener('change', () => loadProfitLossHistory(true));
    }
    if (limitSelect) {
        limitSelect.addEventListener('change', () => loadProfitLossHistory(true));
    }

    // Setup infinite scroll for history table
    setupTableInfiniteScroll({
        tableBodyId: 'history-table-body',
        fetchFunction: () => loadProfitLossHistory(false),
        getHasMore: () => state.tables.history.hasMore,
        getIsLoading: () => state.tables.history.isLoading,
        noMoreDataId: 'history-no-more-data'
    });

    // Initial load
    loadProfitLossHistory(true);
}

/**
 * Load profit/loss history
 * @param {boolean} reset - Reset pagination
 */
async function loadProfitLossHistory(reset = false) {
    if (state.tables.history.isLoading) return;
    if (!reset && !state.tables.history.hasMore) return;

    const tbody = safeGetElement('history-table-body');
    const placeholder = safeGetElement('history-placeholder');
    const loading = safeGetElement('history-loading');
    const loadingMore = safeGetElement('history-loading-more');
    const noMoreData = safeGetElement('history-no-more-data');

    if (!tbody) return;

    state.tables.history.isLoading = true;

    // Show appropriate loading indicator
    if (reset) {
        if (loading) loading.style.display = 'block';
        if (placeholder) placeholder.style.display = 'none';
        if (noMoreData) noMoreData.style.display = 'none';
        state.tables.history.offset = 0;
        state.tables.history.data = [];
    } else {
        if (loadingMore) loadingMore.style.display = 'flex';
        if (noMoreData) noMoreData.style.display = 'none';
    }

    try {
        const filters = {
            strategy: document.getElementById('history-strategy')?.value || 'ALL',
            status: document.getElementById('history-status')?.value || '',
            limit: 50, // Fixed page size for lazy loading
            offset: reset ? 0 : state.tables.history.offset,
            symbol: state.currentFilters.search || ''
        };

        console.log(`📊 Loading P&L history... Reset: ${reset}, Offset: ${filters.offset}`);

        const data = await API.fetchProfitLossHistory(filters);
        const history = data.history || [];
        const hasMore = data.has_more !== undefined ? data.has_more : history.length >= 50;

        console.log(`✅ Loaded ${history.length} P&L records, HasMore: ${hasMore}`);

        // Update state
        if (reset) {
            state.tables.history.data = history;
            state.tables.history.offset = history.length;
        } else {
            state.tables.history.data = state.tables.history.data.concat(history);
            state.tables.history.offset += history.length;
        }
        state.tables.history.hasMore = hasMore;

        // Render all accumulated data
        renderProfitLossHistory(state.tables.history.data, tbody, placeholder);

        // Show "no more data" if we've reached the end
        if (!hasMore && state.tables.history.data.length > 0 && noMoreData) {
            noMoreData.style.display = 'block';
        }
    } catch (error) {
        console.error('Failed to load P&L history:', error);
        if (tbody && reset) {
            tbody.innerHTML = '<tr><td colspan="9" class="text-center p-8 text-accentDanger"><span class="text-2xl block mb-2">⚠️</span>Gagal memuat riwayat trading. Silakan coba lagi.</td></tr>';
        }
        state.tables.history.hasMore = false;
    } finally {
        state.tables.history.isLoading = false;
        if (loading) loading.style.display = 'none';
        if (loadingMore) loadingMore.style.display = 'none';
    }
}

/**
 * Load daily performance
 * @param {boolean} reset - Reset pagination
 */
async function loadPerformance(reset = true) {
    if (state.tables.performance.isLoading) return;
    if (!reset && !state.tables.performance.hasMore) return;

    const tbody = document.getElementById('daily-performance-body');
    if (!tbody) return;

    state.tables.performance.isLoading = true;

    if (reset) {
        tbody.innerHTML = '<tr><td colspan="8" class="text-center p-5">Memuat data...</td></tr>';
        state.tables.performance.offset = 0;
        state.tables.performance.data = [];
    }

    try {
        const offset = reset ? 0 : state.tables.performance.offset;
        const data = await API.fetchDailyPerformance(50, offset);
        const performance = data.performance || [];
        const hasMore = data.has_more !== undefined ? data.has_more : performance.length >= 50;

        console.log(`✅ Loaded ${performance.length} performance records, HasMore: ${hasMore}`);

        // Update state
        if (reset) {
            state.tables.performance.data = performance;
            state.tables.performance.offset = performance.length;
        } else {
            state.tables.performance.data = state.tables.performance.data.concat(performance);
            state.tables.performance.offset += performance.length;
        }
        state.tables.performance.hasMore = hasMore;

        // Render all accumulated data
        renderDailyPerformance(state.tables.performance.data);
    } catch (error) {
        console.error('Failed to load performance:', error);
        if (tbody && reset) {
            tbody.innerHTML = '<tr><td colspan="7" class="text-center p-8 text-accentDanger"><span class="text-2xl block mb-2">⚠️</span>Gagal memuat data performa harian. Silakan coba lagi.</td></tr>';
        }
        state.tables.performance.hasMore = false;
    } finally {
        state.tables.performance.isLoading = false;
    }
}

// Initialize when DOM is ready
if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', init);
} else {
    init();
}
