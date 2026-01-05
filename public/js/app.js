/**
 * Main Application Entry Point
 * Orchestrates all modules and initializes the Whale Radar application
 */

import { CONFIG } from './config.js';
import { debounce, safeGetElement } from './utils.js';
import * as API from './api.js';
import { renderWhaleAlerts, renderRunningPositions, renderSummaryTable, updateStatsTicker } from './render.js';
import { createWhaleAlertSSE, createPatternAnalysisSSE } from './sse-handler.js';
import { initStrategySystem } from './strategy-manager.js';

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
    currentPatternType: 'accumulation'
};

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

    // Initialize strategy system
    initStrategySystem();

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
    const container = document.querySelector('.whale-alerts-section .table-container');
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
    // Implementation would go here
    window.openCandleModal = (symbol) => {
        console.log('Opening candle modal for:', symbol);
        // Candle modal logic
    };
}

/**
 * Setup followup modal
 */
function setupFollowupModal() {
    // Implementation would go here
    window.openFollowupModal = (alertId, symbol, price) => {
        console.log('Opening followup modal for:', alertId, symbol, price);
        // Followup modal logic
    };
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
            document.querySelectorAll('.tab-panel').forEach(panel => {
                panel.classList.remove('active');
            });
            const targetPanel = document.getElementById(target);
            if (targetPanel) targetPanel.classList.add('active');
        });
    });
}

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
 * Start analytics polling
 */
function startAnalyticsPolling() {
    setInterval(() => {
        Promise.all([
            API.fetchMarketIntelligence(),
            API.fetchAnalyticsHub(),
            API.fetchOrderFlow(),
            API.fetchRecentFollowups(),
            API.fetchRunningPositions().then(renderPositions)
        ]).catch(error => {
            console.error('Analytics polling error:', error);
        });
    }, CONFIG.ANALYTICS_POLL_INTERVAL);

    // Stats polling
    setInterval(fetchStats, CONFIG.STATS_POLL_INTERVAL);
}

// Initialize when DOM is ready
if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', init);
} else {
    init();
}
