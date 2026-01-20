/**
 * Main Application Entry Point
 * Orchestrates all modules and initializes the Whale Radar application
 */

import { CONFIG } from './config.js';
import { debounce, safeGetElement, setupTableInfiniteScroll, formatNumber, renderWhaleAlignmentBadge, renderRegimeBadge } from './utils.js?v=2';
import * as API from './api.js';
import { renderRunningPositions, renderSummaryTable, updateStatsTicker, renderStockCorrelations, renderProfitLossHistory, renderMarketIntelligence, renderOrderFlow, renderPatternFeed, renderDailyPerformance } from './render.js';
import { createWhaleAlertSSE, createPatternAnalysisSSE, createCustomPromptSSE } from './sse-handler.js?v=2';
import { initStrategySystem } from './strategy-manager.js?v=2';
import { initWebhookManagement } from './webhook-config.js';

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
        // Critical data - will block on error
        await Promise.all([
            fetchAlerts(true),
            fetchStats(),
            API.fetchAccumulationSummary().then(renderAccumulationSummary),
            API.fetchRunningPositions().then(renderPositions)
        ]);
    } catch (error) {
        console.error('Initial data load error:', error);
    }

    // Setup event listeners
    setupFilterControls();
    setupModals();
    setupAnalyticsTabs();
    setupPatternAnalysis();
    setupInfiniteScroll();
    setupProfitLossHistory();
    setupAccumulationTables();

    // Initialize strategy system
    initStrategySystem();

    // Initialize webhook management
    initWebhookManagement();

    // Setup mobile filter toggle
    setupMobileFilterToggle();

    // Setup running trades toggle
    setupRunningTradesToggle();

    // Connect SSE for real-time updates
    connectWhaleAlertSSE();

    // Start polling for analytics
    startAnalyticsPolling();

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

    try {
        const offset = reset ? 0 : state.currentOffset;
        const data = await API.fetchAlerts(state.currentFilters, offset);

        const alerts = data.data || [];
        state.hasMore = data.has_more || false;

        console.log(`✅ Received ${alerts.length} alerts. Total: ${state.alerts.length + alerts.length}, HasMore: ${state.hasMore}`);

        if (reset) {
            state.alerts = alerts;
            state.currentOffset = alerts.length;
        } else {
            state.alerts = state.alerts.concat(alerts);
            state.currentOffset += alerts.length;
        }

        const tbody = safeGetElement('alerts-table-body');
        renderWhaleAlerts(state.alerts, tbody, loadingDiv);

        // Show "no more data" if we've reached the end
        if (!state.hasMore && state.alerts.length > 0 && noMoreData) {
            noMoreData.style.display = 'block';
        }
    } catch (error) {
        console.error('Failed to fetch alerts:', error);
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
    }
}

/**
 * Refresh all data
 */
async function refreshAllData() {
    // Critical data
    await Promise.all([
        fetchAlerts(true),
        fetchStats()
    ]);

    // Optional analytics - don't block on errors
    API.fetchAnalyticsHub().catch(err => console.warn('Analytics hub unavailable'));
    API.fetchOrderFlow().catch(err => console.warn('Order flow unavailable'));

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

            // Prepend new alert
            state.alerts.unshift(newAlert);
            if (state.alerts.length > CONFIG.MAX_ALERTS_CACHE) {
                state.alerts.pop();
            }
            state.currentOffset = Math.min(state.currentOffset + 1, CONFIG.MAX_ALERTS_CACHE);

            const tbody = safeGetElement('alerts-table-body');
            const loadingDiv = safeGetElement('loading');
            renderWhaleAlerts(state.alerts, tbody, loadingDiv);

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
        return (val / 1_000).toFixed(0) + 'k';
    };

    row.innerHTML = `
        <td class="px-4 py-1.5 text-textMuted">${timeStr}</td>
        <td class="px-4 py-1.5 font-bold text-textPrimary">${trade.symbol}</td>
        <td class="px-4 py-1.5 text-right font-bold ${priceClass}">${trade.price}</td>
        <td class="px-4 py-1.5 text-right text-textSecondary">${trade.volume_lot.toLocaleString('id-ID')}</td>
        <td class="px-4 py-1.5 text-right text-textPrimary">${formatValue(trade.value)}</td>
        <td class="px-4 py-1.5 text-center font-bold ${actionClass}">${trade.action === 'BUY' ? 'B' : 'S'}</td>
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
    const positions = data.positions || [];
    const tbody = safeGetElement('positions-table-body');
    const placeholder = safeGetElement('positions-placeholder');
    renderRunningPositions(positions, tbody, placeholder);
}

/**
 * Setup modals
 */
function setupModals() {
    // Help modal
    setupHelpModal();

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
 * Setup candle modal
 */
function setupCandleModal() {
    const modal = safeGetElement('candle-modal');
    const closeBtn = safeGetElement('candle-modal-close');

    if (closeBtn) {
        closeBtn.addEventListener('click', () => {
            if (modal) modal.classList.add('hidden');
        });
    }

    if (modal) {
        modal.addEventListener('click', (e) => {
            if (e.target === modal) modal.classList.add('hidden');
        });
    }

    // Setup timeframe tabs
    const tabs = document.querySelectorAll('.c-tab');
    tabs.forEach(tab => {
        tab.addEventListener('click', () => {
            const timeframe = tab.dataset.timeframe;
            const symbol = modal.dataset.currentSymbol;

            if (symbol && timeframe) {
                tabs.forEach(t => t.classList.remove('active'));
                tab.classList.add('active');
                fetchAndDisplayCandles(symbol, timeframe);
            }
        });
    });

    // Global function to open modal
    window.openCandleModal = async (symbol) => {
        console.log('Opening candle modal for:', symbol);

        if (!modal) {
            console.error('Candle modal not found');
            return;
        }

        // Update modal title
        const titleEl = safeGetElement('candle-modal-title');
        if (titleEl) titleEl.textContent = `📉 Market Details: ${symbol}`;

        // Store current symbol
        modal.dataset.currentSymbol = symbol;

        // Show modal
        modal.classList.remove('hidden');

        // Reset to default timeframe
        const tabs = document.querySelectorAll('.c-tab');
        tabs.forEach(t => t.classList.remove('active'));
        const defaultTab = document.querySelector('.c-tab[data-timeframe="5m"]');
        if (defaultTab) defaultTab.classList.add('active');

        // Fetch and display candles
        await fetchAndDisplayCandles(symbol, '5m');
    };
}

/**
 * Fetch and display candle data
 * @param {string} symbol - Stock symbol
 * @param {string} timeframe - Timeframe (5m, 15m, 1h, 1d)
 */
async function fetchAndDisplayCandles(symbol, timeframe) {
    const tbody = safeGetElement('candle-list-body');
    if (!tbody) return;

    // Show loading
    tbody.innerHTML = '<tr><td colspan="6" class="text-center p-8 text-textSecondary">Loading candles...</td></tr>';

    try {
        const data = await API.fetchCandles(symbol, timeframe);
        displayCandles(data.candles || []);
    } catch (error) {
        console.error('Failed to fetch candles:', error);
        tbody.innerHTML = '<tr><td colspan="6" class="text-center p-8 text-accentDanger">Failed to load candle data</td></tr>';
    }
}

/**
 * Display candles in the table
 * @param {Array} candles - Array of candle objects
 */
function displayCandles(candles) {
    const tbody = safeGetElement('candle-list-body');
    if (!tbody) return;

    if (candles.length === 0) {
        tbody.innerHTML = '<tr><td colspan="6" class="text-center p-8 text-textSecondary">No candle data available</td></tr>';
        return;
    }

    tbody.innerHTML = '';
    candles.forEach(candle => {
        const row = document.createElement('tr');
        row.className = 'hover:bg-bgHover transition-colors border-b border-borderColor last:border-0';

        // Determine candle color (green for bullish, red for bearish)
        const isBullish = candle.close >= candle.open;
        const priceClass = isBullish ? 'text-accentSuccess' : 'text-accentDanger';

        // Format time with proper error handling - use 'time' field from API
        let time = 'N/A';
        try {
            const timeField = candle.time || candle.timestamp; // Support both field names
            if (timeField) {
                const date = new Date(timeField);
                if (!isNaN(date.getTime())) {
                    time = date.toLocaleString('id-ID', {
                        month: 'short',
                        day: '2-digit',
                        hour: '2-digit',
                        minute: '2-digit'
                    });
                }
            }
        } catch (error) {
            console.error('Error formatting time:', candle.time || candle.timestamp, error);
        }

        // Format trade count if available
        const tradeInfo = candle.trade_count ? ` (${candle.trade_count} trades)` : '';

        row.innerHTML = `
            <td class="px-4 py-2 text-sm text-textMuted" title="Trade Count: ${candle.trade_count || 0}">${time}</td>
            <td class="px-4 py-2 text-right text-textPrimary">${(candle.open || 0).toLocaleString('id-ID')}</td>
            <td class="px-4 py-2 text-right text-accentSuccess">${(candle.high || 0).toLocaleString('id-ID')}</td>
            <td class="px-4 py-2 text-right text-accentDanger">${(candle.low || 0).toLocaleString('id-ID')}</td>
            <td class="px-4 py-2 text-right ${priceClass} font-bold">${(candle.close || 0).toLocaleString('id-ID')}</td>
            <td class="px-4 py-2 text-right text-sm" title="Total Value: Rp ${(candle.total_value || 0).toLocaleString('id-ID')}">${(candle.volume || 0).toLocaleString('id-ID')}</td>
        `;

        tbody.appendChild(row);
    });
}


/**
 * Setup followup modal
 */
function setupFollowupModal() {
    const modal = safeGetElement('followup-modal');
    const closeBtn = safeGetElement('followup-close');

    if (closeBtn) {
        closeBtn.addEventListener('click', () => {
            if (modal) modal.classList.add('hidden');
        });
    }

    if (modal) {
        modal.addEventListener('click', (e) => {
            if (e.target === modal) modal.classList.add('hidden');
        });
    }

    // Global function to open modal
    window.openFollowupModal = async (alertId, symbol, triggerPrice) => {
        console.log('Opening followup modal:', { alertId, symbol, triggerPrice });

        if (!modal) {
            console.error('Followup modal not found');
            return;
        }

        // Update modal title
        const titleEl = safeGetElement('followup-symbol');
        if (titleEl) titleEl.textContent = symbol || 'N/A';

        const triggerEl = safeGetElement('followup-trigger-price');
        if (triggerEl) triggerEl.textContent = triggerPrice ? `Rp ${triggerPrice}` : 'N/A';

        // Show modal
        modal.classList.remove('hidden');

        // Fetch followup data
        try {
            const data = await API.fetchWhaleFollowup(alertId);
            displayFollowupData(data);
        } catch (error) {
            console.error('Failed to fetch followup:', error);
            const container = safeGetElement('followup-data');
            if (container) {
                container.innerHTML = '<div class="p-8 text-center text-accentDanger">Gagal memuat data followup</div>';
            }
        }
    };
}

/**
 * Display followup data in modal
 * @param {Object} data - Followup data
 */
function displayFollowupData(data) {
    if (!data) {
        console.error('No followup data received');
        return;
    }

    // Update modal with followup information
    const symbolEl = document.getElementById('followup-symbol');
    const triggerPriceEl = document.getElementById('followup-trigger-price');
    const followupDataEl = document.getElementById('followup-data');

    if (symbolEl) symbolEl.textContent = data.stock_symbol || 'N/A';
    if (triggerPriceEl) triggerPriceEl.textContent = data.alert_price ? `Rp ${data.alert_price}` : 'N/A';

    if (!followupDataEl) {
        console.error('Followup data container not found');
        return;
    }

    // Create followup content
    const priceChange = data.price_change_pct || 0;
    const priceChangeClass = priceChange >= 0 ? 'text-accentSuccess' : 'text-accentDanger';
    const priceChangeSign = priceChange >= 0 ? '+' : '';

    // Calculate time elapsed since alert
    let timeDiff = 'N/A';
    if (data.alert_time) {
        try {
            const alertTime = new Date(data.alert_time);
            const now = new Date();
            const diffMs = now - alertTime;
            const diffMins = Math.floor(diffMs / 60000);

            if (diffMins < 60) {
                timeDiff = `${diffMins}m`;
            } else {
                const diffHours = Math.floor(diffMins / 60);
                const remainingMins = diffMins % 60;
                timeDiff = `${diffHours}h ${remainingMins}m`;
            }
        } catch (e) {
            console.error('Error calculating time diff:', e);
        }
    }

    const currentPrice = data.current_price || 0;
    const alertPrice = data.alert_price || 0;

    const html = `
        <div class="grid grid-cols-2 gap-4 mb-4">
            <div class="bg-bgCard p-3 rounded border border-borderColor">
                <span class="text-xs text-textSecondary block">Harga Alert</span> 
                <strong class="text-textPrimary">Rp ${alertPrice.toLocaleString('id-ID')}</strong>
            </div>
            <div class="bg-bgCard p-3 rounded border border-borderColor">
                <span class="text-xs text-textSecondary block">Harga Sekarang</span> 
                <strong class="text-textPrimary">Rp ${currentPrice.toLocaleString('id-ID')}</strong>
            </div>
            <div class="bg-bgCard p-3 rounded border border-borderColor">
                <span class="text-xs text-textSecondary block">Perubahan</span> 
                <strong class="${priceChangeClass}">${priceChangeSign}${priceChange.toFixed(2)}%</strong>
            </div>
            <div class="bg-bgCard p-3 rounded border border-borderColor">
                <span class="text-xs text-textSecondary block">Waktu Berlalu</span> 
                <strong class="text-textPrimary">${timeDiff}</strong>
            </div>
        </div>
        <div class="bg-bgCard p-4 rounded border border-borderColor">
            <h4 class="text-sm font-bold text-textPrimary mb-2">📊 Analisis Pergerakan</h4>
            <p class="text-sm text-textSecondary">${data.analysis || 'Data analisis tidak tersedia'}</p>
        </div>
    `;

    followupDataEl.innerHTML = html;
}

/**
 * Render whale alerts table (Compact Mode)
 * @param {Array} alerts - Array of alert objects
 * @param {HTMLElement} tbody - Table body element
 * @param {HTMLElement} loadingDiv - Loading indicator element
 */
function renderWhaleAlerts(alerts, tbody, loadingDiv) {
    if (!tbody) return;

    // Hide loading
    if (loadingDiv) loadingDiv.style.display = 'none';

    if (alerts.length === 0) {
        tbody.innerHTML = '<tr><td colspan="6" class="text-center p-8 text-textSecondary">Belum ada aktivitas whale terdeteksi</td></tr>';
        return;
    }

    tbody.innerHTML = '';

    alerts.forEach(alert => {
        const row = document.createElement('tr');
        row.className = 'hover:bg-bgHover transition-colors border-b border-borderColor last:border-0 text-xs';

        // Render badges (Compact version)
        const whaleBadge = renderWhaleAlignmentBadge(alert);
        const regimeBadge = renderRegimeBadge(alert.market_regime);

        // Format Time (Just HH:mm)
        const date = new Date(alert.timestamp || alert.time);
        const timeStr = date.toLocaleTimeString('id-ID', { hour: '2-digit', minute: '2-digit' });

        // Determine Action & Color
        let actionClass = 'text-textPrimary';
        let actionText = alert.signal_type || 'UNKNOWN';
        let actionCode = actionText.substring(0, 1); // B / S / A / D

        if (actionText === 'BUY' || actionText === 'ACCUMULATION') {
            actionClass = 'text-accentSuccess font-bold';
            actionCode = 'B';
        } else if (actionText === 'SELL' || actionText === 'DISTRIBUTION') {
            actionClass = 'text-accentDanger font-bold';
            actionCode = 'S';
        }

        // Format Value (Compact: M/B)
        const valueStr = formatNumber(alert.total_value);

        // RAW PRICE FORMAT
        const priceStr = alert.trigger_price || alert.price || 0;

        row.innerHTML = `
            <td class="px-2 py-1.5 whitespace-nowrap text-textMuted font-mono w-[15%] text-left">${timeStr}</td>
            <td class="px-2 py-1.5 whitespace-nowrap w-[15%] text-left">
                <span class="font-bold text-textPrimary">${alert.stock_symbol}</span>
            </td>
            <td class="px-2 py-1.5 text-center font-bold w-[10%] ${actionClass}">${actionCode}</td>
            <td class="px-2 py-1.5 text-right font-mono font-bold text-textPrimary w-[15%]">${priceStr}</td>
            <td class="px-2 py-1.5 text-right text-textSecondary w-[15%]">${valueStr}</td>
            <td class="px-2 py-1.5 w-[20%] text-left">
                <div class="flex gap-1 items-center justify-start flex-wrap">
                    ${whaleBadge ? whaleBadge.replace('text-[10px]', 'text-[9px] py-0 px-1') : ''}
                    ${regimeBadge ? regimeBadge.replace('text-[10px]', 'text-[9px] py-0 px-1') : ''}
                </div>
            </td>
            <td class="px-2 py-1.5 text-right whitespace-nowrap w-[10%]">
                <button onclick="openFollowupModal('${alert.id}', '${alert.stock_symbol}', ${alert.trigger_price})" class="p-1 hover:bg-bgSecondary rounded text-accentInfo transition-colors" title="Lihat Followup">
                    🔍
                </button>
                <button onclick="openCandleModal('${alert.stock_symbol}')" class="p-1 hover:bg-bgSecondary rounded text-accentWarning transition-colors ml-1" title="Lihat Chart">
                    📈
                </button>
            </td>
        `;

        tbody.appendChild(row);
    });
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
                effectivenessBody.innerHTML = '<tr><td colspan="6" class="text-center p-4 text-textSecondary">Belum ada data historis</td></tr>';
            } else {
                effectivenessBody.innerHTML = effs.map(e => {
                    const wrClass = e.win_rate >= 50 ? 'text-accentSuccess' : (e.win_rate < 40 ? 'text-accentDanger' : 'text-textSecondary');
                    const evClass = e.expected_value > 0 ? 'text-accentSuccess' : (e.expected_value < 0 ? 'text-accentDanger' : '');
                    return `
                        <tr class="hover:bg-bgHover transition-colors border-b border-borderColor last:border-0">
                            <td class="p-3"><strong>${e.strategy}</strong></td>
                            <td class="p-3">${e.market_regime}</td>
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
 * Setup pattern analysis
 */
function setupPatternAnalysis() {
    const tabs = document.querySelectorAll('.tab-btn');
    const startBtn = safeGetElement('start-analysis-btn');
    const stopBtn = safeGetElement('stop-analysis-btn');

    tabs.forEach(tab => {
        tab.addEventListener('click', () => {
            tabs.forEach(t => t.classList.remove('active'));
            tab.classList.add('active');
            state.currentPatternType = tab.dataset.type;

            const symbolInput = safeGetElement('symbol-input-container');
            const customPromptContainer = safeGetElement('custom-prompt-container');

            if (symbolInput) {
                symbolInput.style.display = state.currentPatternType === 'symbol' ? 'block' : 'none';
            }

            if (customPromptContainer) {
                customPromptContainer.style.display = state.currentPatternType === 'custom' ? 'block' : 'none';
            }
        });
    });

    if (startBtn) {
        startBtn.addEventListener('click', () => startPatternAnalysis());
    }

    if (stopBtn) {
        stopBtn.addEventListener('click', () => stopPatternAnalysis());
    }
}

/**
 * Start pattern analysis
 */
function startPatternAnalysis() {
    const outputDiv = safeGetElement('llm-stream-output');
    const statusBadge = safeGetElement('llm-status');
    const startBtn = safeGetElement('start-analysis-btn');
    const stopBtn = safeGetElement('stop-analysis-btn');

    let symbol = '';
    let customPrompt = '';
    let customSymbols = [];
    let hoursBack = 24;
    let includeData = 'alerts,regimes';

    if (state.currentPatternType === 'symbol') {
        symbol = document.getElementById('symbol-input')?.value.trim().toUpperCase() || '';
        if (!symbol) {
            if (outputDiv) {
                outputDiv.innerHTML = '<div class="flex flex-col items-center justify-center p-8 h-full text-textSecondary"><span class="text-4xl mb-4">⚠️</span><p class="text-accentDanger">Silakan masukkan kode saham terlebih dahulu</p></div>';
            }
            return;
        }
    } else if (state.currentPatternType === 'custom') {
        customPrompt = document.getElementById('custom-prompt-input')?.value.trim() || '';
        if (!customPrompt) {
            if (outputDiv) {
                outputDiv.innerHTML = '<div class="flex flex-col items-center justify-center p-8 h-full text-textSecondary"><span class="text-4xl mb-4">⚠️</span><p class="text-accentDanger">Silakan masukkan prompt terlebih dahulu</p></div>';
            }
            return;
        }

        const symbolsInput = document.getElementById('custom-symbols-input')?.value.trim() || '';
        if (symbolsInput) {
            customSymbols = symbolsInput.split(',').map(s => s.trim().toUpperCase()).filter(s => s);
        }

        hoursBack = parseInt(document.getElementById('custom-hours-input')?.value || '24');
        includeData = document.getElementById('custom-data-type')?.value || 'alerts,regimes';
    }

    if (startBtn) startBtn.style.display = 'none';
    if (stopBtn) stopBtn.style.display = 'flex';
    if (statusBadge) {
        statusBadge.textContent = 'Streaming...';
        statusBadge.className = 'px-2 py-1 rounded-full text-xs font-bold bg-accentPrimary/20 text-accentPrimary animate-pulse';
    }

    if (outputDiv) outputDiv.innerHTML = '<div class="flex items-center justify-center p-4 text-textSecondary animate-pulse">🤖 Menganalisis data...</div>';

    let streamText = '';

    if (state.currentPatternType === 'custom') {
        state.patternSSE = createCustomPromptSSE(customPrompt, customSymbols, hoursBack, includeData, {
            onChunk: (chunk) => {
                if (streamText === '' && outputDiv) {
                    outputDiv.innerHTML = '<div class="bg-surface/50 rounded-lg p-4 border border-white/5"><div class="prose prose-invert max-w-none text-textPrimary leading-relaxed streaming-text"></div></div>';
                }

                streamText += chunk;
                const htmlContent = marked.parse(streamText);

                const textContainer = outputDiv?.querySelector('.streaming-text');
                if (textContainer) {
                    textContainer.innerHTML = `${htmlContent}<span class="inline-block w-2 h-4 bg-accentPrimary align-middle ml-1 animate-pulse"></span>`;
                }

                if (outputDiv) outputDiv.scrollTop = outputDiv.scrollHeight;
            },
            onDone: () => {
                const htmlContent = marked.parse(streamText);
                if (outputDiv) {
                    outputDiv.innerHTML = `<div class="bg-surface/50 rounded-lg p-4 border border-white/5"><div class="prose prose-invert max-w-none text-textPrimary leading-relaxed streaming-text">${htmlContent}</div></div>`;
                }

                if (statusBadge) {
                    statusBadge.textContent = 'Completed';
                    statusBadge.className = 'px-2 py-1 rounded-full text-xs font-bold bg-accentSuccess/20 text-accentSuccess';
                }

                if (startBtn) startBtn.style.display = 'flex';
                if (stopBtn) stopBtn.style.display = 'none';
            },
            onError: () => {
                if (statusBadge) {
                    statusBadge.textContent = 'Error';
                    statusBadge.className = 'px-2 py-1 rounded-full text-xs font-bold bg-accentDanger/20 text-accentDanger';
                }
            }
        });
    } else {
        state.patternSSE = createPatternAnalysisSSE(state.currentPatternType, symbol, {
            onChunk: (chunk) => {
                if (streamText === '' && outputDiv) {
                    outputDiv.innerHTML = '<div class="bg-surface/50 rounded-lg p-4 border border-white/5"><div class="prose prose-invert max-w-none text-textPrimary leading-relaxed streaming-text"></div></div>';
                }

                streamText += chunk;
                const htmlContent = marked.parse(streamText);

                const textContainer = outputDiv?.querySelector('.streaming-text');
                if (textContainer) {
                    textContainer.innerHTML = `${htmlContent}<span class="inline-block w-2 h-4 bg-accentPrimary align-middle ml-1 animate-pulse"></span>`;
                }

                if (outputDiv) outputDiv.scrollTop = outputDiv.scrollHeight;
            },
            onDone: () => {
                const htmlContent = marked.parse(streamText);
                if (outputDiv) {
                    outputDiv.innerHTML = `<div class="bg-surface/50 rounded-lg p-4 border border-white/5"><div class="prose prose-invert max-w-none text-textPrimary leading-relaxed streaming-text">${htmlContent}</div></div>`;
                }

                if (statusBadge) {
                    statusBadge.textContent = 'Completed';
                    statusBadge.className = 'px-2 py-1 rounded-full text-xs font-bold bg-accentSuccess/20 text-accentSuccess';
                }

                if (startBtn) startBtn.style.display = 'flex';
                if (stopBtn) stopBtn.style.display = 'none';
            },
            onError: () => {
                if (statusBadge) {
                    statusBadge.textContent = 'Error';
                    statusBadge.className = 'px-2 py-1 rounded-full text-xs font-bold bg-accentDanger/20 text-accentDanger';
                }
            }
        });
    }
}

/**
 * Stop pattern analysis
 */
function stopPatternAnalysis() {
    if (state.patternSSE) {
        state.patternSSE.close();
        state.patternSSE = null;
    }

    const startBtn = safeGetElement('start-analysis-btn');
    const stopBtn = safeGetElement('stop-analysis-btn');
    const statusBadge = safeGetElement('llm-status');

    if (startBtn) startBtn.style.display = 'flex';
    if (stopBtn) stopBtn.style.display = 'none';
    if (statusBadge) {
        statusBadge.textContent = 'Stopped';
        statusBadge.className = 'status-badge';
    }
}

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
            API.fetchMarketRegime(symbol).then(renderMarketIntelligence).catch(() => {
                renderMarketIntelligence({ regime: 'UNKNOWN', confidence: 0 });
            }),
            API.fetchRunningPositions().then(renderPositions).catch(() => null)
        ];

        // Fetch market intelligence including baseline and patterns
        promises.push(
            API.fetchMarketIntelligence(symbol).then(data => {
                // Render patterns
                renderPatternFeed(data.patterns);

                // Render baseline data
                if (data.baseline) {
                    renderMarketIntelligence({ baseline: data.baseline });
                }
            }).catch(() => null)
        );

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

    // NEW: Regime filter
    const regimeSelect = safeGetElement('history-regime');
    if (regimeSelect) {
        regimeSelect.addEventListener('change', () => loadProfitLossHistory(true));
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
            regime: document.getElementById('history-regime')?.value || 'ALL',
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
        if (tbody) {
            tbody.innerHTML = '<tr><td colspan="11" class="text-center p-8 text-accentDanger">Gagal memuat riwayat P&L</td></tr>';
        }
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
        if (tbody) {
            tbody.innerHTML = '<tr><td colspan="8" class="text-center p-8 text-accentDanger">Gagal memuat data</td></tr>';
        }
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
