/**
 * Main Application Entry Point
 * Orchestrates all modules and initializes the Whale Radar application
 */

import { CONFIG } from './config.js';
import { debounce, safeGetElement } from './utils.js';
import * as API from './api.js';
import { renderWhaleAlerts, renderRunningPositions, renderSummaryTable, updateStatsTicker, renderStockCorrelations, renderProfitLossHistory, renderMarketIntelligence, renderOrderFlow, renderPatternFeed, renderDailyPerformance } from './render.js';
import { createWhaleAlertSSE, createPatternAnalysisSSE } from './sse-handler.js';
import { initStrategySystem } from './strategy-manager.js';
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
    statsIntervalId: null
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

    // Initialize strategy system
    initStrategySystem();

    // Initialize webhook management
    initWebhookManagement();

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
    if (loadingDiv) loadingDiv.style.display = 'block';

    try {
        const offset = reset ? 0 : state.currentOffset;
        const data = await API.fetchAlerts(state.currentFilters, offset);

        const alerts = data.data || [];
        state.hasMore = data.has_more || false;

        if (reset) {
            state.alerts = alerts;
            state.currentOffset = alerts.length;
        } else {
            state.alerts = state.alerts.concat(alerts);
            state.currentOffset += alerts.length;
        }

        const tbody = safeGetElement('alerts-table-body');
        renderWhaleAlerts(state.alerts, tbody, loadingDiv);
    } catch (error) {
        console.error('Failed to fetch alerts:', error);
    } finally {
        state.isLoading = false;
        if (loadingDiv) loadingDiv.style.display = 'none';
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
        if (element && element.value !== 'ALL' && element.value !== '0') {
            element.style.borderColor = '#0ECB81';
            element.style.boxShadow = '0 0 0 2px rgba(14, 203, 129, 0.2)';
        } else if (element) {
            element.style.borderColor = '';
            element.style.boxShadow = '';
        }
    });
}

/**
 * Setup infinite scroll
 */
function setupInfiniteScroll() {
    // Find the first table-wrapper in the whale alerts section (first card with alerts-table-body)
    const alertsTable = document.getElementById('alerts-table-body');
    const container = alertsTable?.closest('.table-wrapper');
    if (container) {
        container.addEventListener('scroll', () => {
            const { scrollTop, scrollHeight, clientHeight } = container;
            if (scrollHeight - scrollTop - clientHeight < CONFIG.SCROLL_THRESHOLD && state.hasMore && !state.isLoading) {
                fetchAlerts(false);
            }
        });
    }
}

/**
 * Connect to whale alert SSE
 */
function connectWhaleAlertSSE() {
    state.whaleSSE = createWhaleAlertSSE(
        (newAlert) => {
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
        (error) => {
            console.error('SSE Error:', error);
        }
    );
}

/**
 * Render accumulation summary
 * @param {Object} data - Summary data
 */
function renderAccumulationSummary(data) {
    const accumulation = data.accumulation || [];
    const distribution = data.distribution || [];

    // Update counters
    const accCount = safeGetElement('accumulation-count');
    const distCount = safeGetElement('distribution-count');
    if (accCount) accCount.textContent = accumulation.length;
    if (distCount) distCount.textContent = distribution.length;

    // Render tables
    const accTbody = safeGetElement('accumulation-table-body');
    const accPlaceholder = safeGetElement('accumulation-placeholder');
    renderSummaryTable('accumulation', accumulation, accTbody, accPlaceholder);

    const distTbody = safeGetElement('distribution-table-body');
    const distPlaceholder = safeGetElement('distribution-placeholder');
    renderSummaryTable('distribution', distribution, distTbody, distPlaceholder);
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

    const closeModal = () => modal.classList.remove('show');

    helpBtn.addEventListener('click', () => modal.classList.add('show'));
    if (modalClose) modalClose.addEventListener('click', closeModal);
    if (modalGotIt) modalGotIt.addEventListener('click', closeModal);

    modal.addEventListener('click', (e) => {
        if (e.target === modal) closeModal();
    });

    document.addEventListener('keydown', (e) => {
        if (e.key === 'Escape' && modal.classList.contains('show')) {
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
            if (modal) modal.classList.remove('show');
        });
    }

    if (modal) {
        modal.addEventListener('click', (e) => {
            if (e.target === modal) modal.classList.remove('show');
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
        modal.classList.add('show');

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
    tbody.innerHTML = '<tr><td colspan="6" style="text-align: center; padding: 2rem; color: var(--text-secondary);">Loading candles...</td></tr>';

    try {
        const data = await API.fetchCandles(symbol, timeframe);
        displayCandles(data.candles || []);
    } catch (error) {
        console.error('Failed to fetch candles:', error);
        tbody.innerHTML = '<tr><td colspan="6" style="text-align: center; padding: 2rem; color: var(--accent-sell);">Failed to load candle data</td></tr>';
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
        tbody.innerHTML = '<tr><td colspan="6" style="text-align: center; padding: 2rem; color: var(--text-secondary);">No candle data available</td></tr>';
        return;
    }

    tbody.innerHTML = '';
    candles.forEach(candle => {
        const row = document.createElement('tr');

        // Determine candle color (green for bullish, red for bearish)
        const isBullish = candle.close >= candle.open;
        const priceClass = isBullish ? 'diff-positive' : 'diff-negative';

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
            <td style="font-size: 0.85em;" title="Trade Count: ${candle.trade_count || 0}">${time}</td>
            <td class="text-right">${(candle.open || 0).toLocaleString('id-ID')}</td>
            <td class="text-right diff-positive">${(candle.high || 0).toLocaleString('id-ID')}</td>
            <td class="text-right diff-negative">${(candle.low || 0).toLocaleString('id-ID')}</td>
            <td class="text-right ${priceClass}" style="font-weight: 600;">${(candle.close || 0).toLocaleString('id-ID')}</td>
            <td class="text-right" style="font-size: 0.85em;" title="Total Value: Rp ${(candle.total_value || 0).toLocaleString('id-ID')}">${(candle.volume || 0).toLocaleString('id-ID')}</td>
        `;

        // Add hover effect
        row.addEventListener('mouseenter', () => {
            row.style.backgroundColor = isBullish ? 'rgba(14, 203, 129, 0.05)' : 'rgba(246, 70, 93, 0.05)';
        });
        row.addEventListener('mouseleave', () => {
            row.style.backgroundColor = '';
        });

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
            if (modal) modal.classList.remove('show');
        });
    }

    if (modal) {
        modal.addEventListener('click', (e) => {
            if (e.target === modal) modal.classList.remove('show');
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
        modal.classList.add('show');

        // Fetch followup data
        try {
            const data = await API.fetchWhaleFollowup(alertId);
            displayFollowupData(data);
        } catch (error) {
            console.error('Failed to fetch followup:', error);
            const container = safeGetElement('followup-data');
            if (container) {
                container.innerHTML = '<div class="placeholder"><p style="color: var(--accent-sell);">Gagal memuat data followup</p></div>';
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
    const priceChangeClass = priceChange >= 0 ? 'diff-positive' : 'diff-negative';
    const priceChangeSign = priceChange >= 0 ? '+' : '';

    const timeDiff = data.time_since_alert || 'N/A';
    const currentPrice = data.current_price || 0;
    const alertPrice = data.alert_price || 0;

    const html = `
        <div class="followup-stats">
            <div class="f-stat">
                <span>Harga Alert:</span> 
                <strong>Rp ${alertPrice.toLocaleString('id-ID')}</strong>
            </div>
            <div class="f-stat">
                <span>Harga Sekarang:</span> 
                <strong>Rp ${currentPrice.toLocaleString('id-ID')}</strong>
            </div>
            <div class="f-stat">
                <span>Perubahan:</span> 
                <strong class="${priceChangeClass}">${priceChangeSign}${priceChange.toFixed(2)}%</strong>
            </div>
            <div class="f-stat">
                <span>Waktu Berlalu:</span> 
                <strong>${timeDiff}</strong>
            </div>
        </div>
        <div class="followup-analysis">
            <h4>📊 Analisis Pergerakan</h4>
            <p>${data.analysis || 'Data analisis tidak tersedia'}</p>
        </div>
    `;

    followupDataEl.innerHTML = html;
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
                loadPerformance();
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
    if (evList) evList.innerHTML = '<div class="placeholder-small">Memuat data...</div>';
    if (thresholdList) thresholdList.innerHTML = '<div class="placeholder-small">Memuat data...</div>';
    if (effectivenessBody) effectivenessBody.innerHTML = '<tr><td colspan="6" class="text-center">Memuat data...</td></tr>';

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
                evList.innerHTML = '<div class="placeholder-small">Belum ada data historis</div>';
            } else {
                evList.innerHTML = evs.map(ev => {
                    const evClass = ev.expected_value > 0 ? 'diff-positive' : (ev.expected_value < 0 ? 'diff-negative' : '');
                    const recClass = ev.recommendation === 'STRONG' ? 'diff-positive' :
                        (ev.recommendation === 'AVOID' ? 'diff-negative' : '');
                    return `
                        <div style="display: flex; justify-content: space-between; align-items: center; padding: 0.5rem 0; border-bottom: 1px solid var(--border-color);">
                            <span style="font-weight: 600;">${ev.strategy}</span>
                            <div style="text-align: right;">
                                <span class="${evClass}" style="font-weight: 600;">${ev.expected_value > 0 ? '+' : ''}${ev.expected_value.toFixed(4)}</span>
                                <span class="${recClass}" style="font-size: 0.75em; margin-left: 0.5rem; padding: 2px 6px; background: rgba(255,255,255,0.1); border-radius: 4px;">${ev.recommendation}</span>
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
                thresholdList.innerHTML = '<div class="placeholder-small">Belum ada data historis</div>';
            } else {
                thresholdList.innerHTML = thresholds.map(t => `
                    <div style="display: flex; justify-content: space-between; align-items: center; padding: 0.5rem 0; border-bottom: 1px solid var(--border-color);">
                        <span style="font-weight: 600;">${t.strategy}</span>
                        <div style="text-align: right;">
                            <span style="color: var(--accent-gold); font-weight: 600;">${(t.recommended_min_conf * 100).toFixed(0)}%</span>
                            <span style="font-size: 0.75em; color: var(--text-secondary); margin-left: 0.5rem;">(${t.sample_size} sinyal)</span>
                        </div>
                    </div>
                `).join('');
            }
        }

        // Render Effectiveness Table
        if (effectivenessBody) {
            const effs = effectivenessData.effectiveness || [];
            if (effs.length === 0) {
                effectivenessBody.innerHTML = '<tr><td colspan="6" class="text-center">Belum ada data historis</td></tr>';
            } else {
                effectivenessBody.innerHTML = effs.map(e => {
                    const wrClass = e.win_rate >= 50 ? 'diff-positive' : (e.win_rate < 40 ? 'diff-negative' : '');
                    const evClass = e.expected_value > 0 ? 'diff-positive' : (e.expected_value < 0 ? 'diff-negative' : '');
                    return `
                        <tr>
                            <td><strong>${e.strategy}</strong></td>
                            <td>${e.market_regime}</td>
                            <td class="text-right">${e.total_signals}</td>
                            <td class="text-right ${wrClass}">${e.win_rate.toFixed(1)}%</td>
                            <td class="text-right">${e.avg_profit_pct.toFixed(2)}%</td>
                            <td class="text-right ${evClass}">${e.expected_value > 0 ? '+' : ''}${e.expected_value.toFixed(4)}</td>
                        </tr>
                    `;
                }).join('');
            }
        }

        console.log('✅ Strategy optimization data loaded successfully');
    } catch (error) {
        console.error('Failed to load optimization data:', error);
        if (evList) evList.innerHTML = '<div class="placeholder-small" style="color: var(--accent-sell);">Gagal memuat data</div>';
        if (thresholdList) thresholdList.innerHTML = '<div class="placeholder-small" style="color: var(--accent-sell);">Gagal memuat data</div>';
        if (effectivenessBody) effectivenessBody.innerHTML = '<tr><td colspan="6" class="text-center" style="color: var(--accent-sell);">Gagal memuat data</td></tr>';
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
            if (symbolInput) {
                symbolInput.style.display = state.currentPatternType === 'symbol' ? 'block' : 'none';
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
    if (state.currentPatternType === 'symbol') {
        symbol = document.getElementById('symbol-input')?.value.trim().toUpperCase() || '';
        if (!symbol) {
            if (outputDiv) {
                outputDiv.innerHTML = '<div class="placeholder"><span class="placeholder-icon">⚠️</span><p style="color: var(--accent-sell);">Silakan masukkan kode saham terlebih dahulu</p></div>';
            }
            return;
        }
    }

    if (startBtn) startBtn.style.display = 'none';
    if (stopBtn) stopBtn.style.display = 'flex';
    if (statusBadge) {
        statusBadge.textContent = 'Streaming...';
        statusBadge.className = 'status-badge streaming';
    }

    if (outputDiv) outputDiv.innerHTML = '<div class="stream-loading">🤖 Menganalisis data...</div>';

    let streamText = '';
    state.patternSSE = createPatternAnalysisSSE(state.currentPatternType, symbol, {
        onChunk: (chunk) => {
            if (streamText === '' && outputDiv) {
                outputDiv.innerHTML = '<div class="message-bubble"><div class="streaming-text"></div></div>';
            }

            streamText += chunk;
            const htmlContent = marked.parse(streamText);

            const textContainer = outputDiv?.querySelector('.streaming-text');
            if (textContainer) {
                textContainer.innerHTML = `${htmlContent}<span class="streaming-cursor"></span>`;
            }

            if (outputDiv) outputDiv.scrollTop = outputDiv.scrollHeight;
        },
        onDone: () => {
            const htmlContent = marked.parse(streamText);
            if (outputDiv) {
                outputDiv.innerHTML = `<div class="message-bubble"><div class="streaming-text">${htmlContent}</div></div>`;
            }

            if (statusBadge) {
                statusBadge.textContent = 'Completed';
                statusBadge.className = 'status-badge';
            }

            if (startBtn) startBtn.style.display = 'flex';
            if (stopBtn) stopBtn.style.display = 'none';
        },
        onError: () => {
            if (statusBadge) {
                statusBadge.textContent = 'Error';
                statusBadge.className = 'status-badge error';
            }
        }
    });
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

        // OPTIMIZATION: Only fetch data relevant to active view
        const promises = [
            // Always fetch these (critical for main dashboard)
            API.fetchOrderFlow().then(renderOrderFlow).catch(() => null),
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
                const header = `<div style="margin-bottom:10px; font-weight:600; color:var(--text-secondary); font-size:0.9em;">${title}</div>`;
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
                <div class="placeholder-small">
                    <p style="color: var(--accent-sell);">Gagal memuat data korelasi</p>
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
            loadProfitLossHistory();
            refreshBtn.style.transform = 'rotate(360deg)';
            setTimeout(() => refreshBtn.style.transform = '', 300);
        });
    }

    // Auto-load on filter change
    if (strategySelect) {
        strategySelect.addEventListener('change', () => loadProfitLossHistory());
    }
    if (statusSelect) {
        statusSelect.addEventListener('change', () => loadProfitLossHistory());
    }
    if (limitSelect) {
        limitSelect.addEventListener('change', () => loadProfitLossHistory());
    }
}

/**
 * Load profit/loss history
 */
async function loadProfitLossHistory() {
    const tbody = safeGetElement('history-table-body');
    const placeholder = safeGetElement('history-placeholder');
    const loading = safeGetElement('history-loading');

    if (!tbody) return;

    // Show loading
    if (loading) loading.style.display = 'block';
    if (placeholder) placeholder.style.display = 'none';

    try {
        const filters = {
            strategy: document.getElementById('history-strategy')?.value || 'ALL',
            status: document.getElementById('history-status')?.value || '',
            limit: parseInt(document.getElementById('history-limit')?.value || '100'),
            symbol: state.currentFilters.search || '' // Use symbol from main search if any
        };

        console.log('📊 Loading P&L history with filters:', filters);

        const data = await API.fetchProfitLossHistory(filters);
        const history = data.history || [];

        console.log(`✅ Loaded ${history.length} P&L records`);

        renderProfitLossHistory(history, tbody, placeholder);
    } catch (error) {
        console.error('Failed to load P&L history:', error);
        if (tbody) {
            tbody.innerHTML = '<tr><td colspan="11" style="text-align: center; padding: 2rem; color: var(--accent-sell);">Gagal memuat riwayat P&L</td></tr>';
        }
    } finally {
        if (loading) loading.style.display = 'none';
    }
}

/**
 * Load daily performance
 */
async function loadPerformance() {
    const tbody = document.getElementById('daily-performance-body');
    if (tbody) {
        tbody.innerHTML = '<tr><td colspan="8" class="text-center" style="padding: 20px;">Memuat data...</td></tr>';
    }

    try {
        const data = await API.fetchDailyPerformance();
        // Backend returns { performance: [...] }
        renderDailyPerformance(data.performance || []);
    } catch (error) {
        console.error('Failed to load performance:', error);
        if (tbody) {
            tbody.innerHTML = '<tr><td colspan="8" class="text-center" style="padding: 20px; color: var(--accent-sell);">Gagal memuat data</td></tr>';
        }
    }
}

// Initialize when DOM is ready
if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', init);
} else {
    init();
}
