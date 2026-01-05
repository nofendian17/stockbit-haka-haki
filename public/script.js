// ===== CONFIGURATION CONSTANTS =====
const CONFIG = {
    API_BASE: '/api',
    PAGE_SIZE: 50,
    MAX_ALERTS_CACHE: 200,
    STATS_POLL_INTERVAL: 10000, // 10 seconds
    SCROLL_THRESHOLD: 200, // pixels from bottom to trigger load
    CURRENCY_BILLION_THRESHOLD: 1_000_000_000,
    CURRENCY_MILLION_THRESHOLD: 1_000_000,
    TIME_SECOND: 1000,
    TIME_MINUTE: 60,
    TIME_HOUR: 3600,
    ANALYTICS_POLL_INTERVAL: 30000, // 30 seconds
};

// Configure marked.js for better analysis formatting
if (typeof marked !== 'undefined') {
    marked.use({
        breaks: true,
        gfm: true,
        headerIds: false,
        mangle: false
    });
}

const API_BASE = CONFIG.API_BASE;
// ===== FORMATTERS =====
const formatCurrency = (val) => {
    // Compact format for large values (e.g. 1.5M, 2.3B)
    if (val >= CONFIG.CURRENCY_BILLION_THRESHOLD) {
        return 'Rp ' + (val / CONFIG.CURRENCY_BILLION_THRESHOLD).toLocaleString('id-ID', { maximumFractionDigits: 1 }) + ' B';
    }
    if (val >= CONFIG.CURRENCY_MILLION_THRESHOLD) {
        return 'Rp ' + (val / CONFIG.CURRENCY_MILLION_THRESHOLD).toLocaleString('id-ID', { maximumFractionDigits: 1 }) + ' M';
    }
    return new Intl.NumberFormat('id-ID', { style: 'currency', currency: 'IDR', maximumFractionDigits: 0 }).format(val);
};

const formatNumber = (val) => {
    return new Intl.NumberFormat('id-ID').format(val);
};

const formatTime = (isoString) => {
    const date = new Date(isoString);
    const now = new Date();
    const diffMs = now - date;
    const diffSec = Math.floor(diffMs / CONFIG.TIME_SECOND);

    if (diffSec < CONFIG.TIME_MINUTE) return `${diffSec} detik lalu`;
    if (diffSec < CONFIG.TIME_HOUR) return `${Math.floor(diffSec / CONFIG.TIME_MINUTE)} menit lalu`;

    return date.toLocaleTimeString('id-ID', { hour: '2-digit', minute: '2-digit' });
};

const formatPercent = (val) => {
    return (val || 0).toFixed(1) + '%';
};

// ===== STATE =====
let alerts = [];
let stats = {};
let currentOffset = 0;
let isLoading = false;
let hasMore = true;
let currentFilters = {
    search: '',
    action: 'ALL',
    amount: 0,
    board: 'ALL'
};

// ===== API FUNCTIONS =====
function buildFilterQuery() {
    const params = new URLSearchParams();

    // Send all filters to backend
    if (currentFilters.search) {
        params.append('symbol', currentFilters.search);
    }
    if (currentFilters.action !== 'ALL') {
        params.append('action', currentFilters.action); // FIXED: Changed from 'type' to 'action'
    }
    if (currentFilters.board !== 'ALL') {
        params.append('board', currentFilters.board);
    }
    if (currentFilters.amount > 0) {
        params.append('min_amount', currentFilters.amount);
    }

    return params.toString();
}
async function fetchAlerts(reset = false) {
    if (isLoading) return;
    if (!reset && !hasMore) return;

    isLoading = true;
    const loadingDiv = document.getElementById('loading');
    if (loadingDiv) loadingDiv.style.display = 'block';

    try {
        const offset = reset ? 0 : currentOffset;
        const filterQuery = buildFilterQuery();
        const url = `${API_BASE}/whales?limit=${CONFIG.PAGE_SIZE}&offset=${offset}${filterQuery ? '&' + filterQuery : ''}`;
        const res = await fetch(url);
        const response = await res.json();

        // Handle new paginated response format
        const data = response.data || [];
        hasMore = response.has_more || false;

        if (reset) {
            alerts = data;
            currentOffset = data.length;
        } else {
            alerts = alerts.concat(data);
            currentOffset += data.length;
        }

        renderAlerts();
        updateStatsTicker();
    } catch (err) {
        console.error("Failed to fetch alerts:", err);
    } finally {
        isLoading = false;
        if (loadingDiv) loadingDiv.style.display = 'none';
    }
}

async function refreshAllData() {
    fetchAlerts(true); // true = reset mode
    fetchStats();
    fetchAnalyticsHubData();
    fetchOrderFlow();
    fetchMarketIntelligence();
}

async function fetchStats() {
    const symbol = currentFilters.search;
    let url = `${API_BASE}/whales/stats`;
    if (symbol) {
        url += `?symbol=${symbol}`;
    }

    try {
        const res = await fetch(url);
        stats = await res.json();
        updateStatsTicker();
    } catch (err) {
        console.error("Failed to fetch stats:", err);
    }
}

// ===== RENDERING FUNCTIONS =====
function renderAlerts() {
    const tbody = document.getElementById('alerts-table-body');
    const loadingDiv = document.getElementById('loading');

    // All filtering now done server-side!
    // No need for client-side filtering anymore

    // Reset
    tbody.innerHTML = '';

    // Use all data from server (already filtered)
    const filtered = alerts;

    if (filtered.length === 0) {
        if (loadingDiv) loadingDiv.innerText = 'Tidak ada alert yang sesuai filter.';
        if (loadingDiv) loadingDiv.style.display = 'block';
        return;
    } else {
        if (loadingDiv) loadingDiv.style.display = 'none';
    }

    filtered.forEach(alert => {
        const row = document.createElement('tr');
        row.className = 'clickable-row';
        row.onclick = () => openFollowupModal(alert.id, alert.stock_symbol, alert.trigger_price || 0);

        let badgeClass = 'unknown';
        if (alert.action === 'BUY') badgeClass = 'buy';
        if (alert.action === 'SELL') badgeClass = 'sell';

        const actionText = alert.action === 'BUY' ? 'BELI' : alert.action === 'SELL' ? 'JUAL' : alert.action;

        // Use snake_case field names from JSON
        const price = alert.trigger_price || 0;
        const volume = alert.trigger_volume_lots || 0;
        const val = alert.trigger_value || 0;

        // Price difference calculation
        const avgPrice = alert.avg_price || 0;
        let priceDiff = '';
        if (avgPrice > 0 && price > 0) {
            const pct = ((price - avgPrice) / avgPrice) * 100;
            const sign = pct >= 0 ? '+' : '';
            const type = pct >= 0 ? 'diff-positive' : 'diff-negative';
            priceDiff = `<span class="${type}" title="vs Avg: ${formatNumber(avgPrice)}">(${sign}${pct.toFixed(1)}%)</span>`;
        }

        // Z-Score and anomaly detection
        const zScore = alert.z_score || 0;
        const volumeVsAvg = alert.volume_vs_avg_pct || 0;

        // Enhanced anomaly HTML with more details
        let anomalyHtml = '';
        if (zScore >= 3.0) {
            const anomalyLevel = zScore >= 5.0 ? '🔴 Ekstrem' : zScore >= 4.0 ? '🟠 Tinggi' : '🟡 Sedang';
            anomalyHtml = `<span class="table-anomaly" title="Skor Anomali: ${zScore.toFixed(2)} | Volume: ${volumeVsAvg.toFixed(0)}% vs Rata-rata">${anomalyLevel}</span>`;
        } else if (volumeVsAvg >= 500) {
            anomalyHtml = `<span class="table-anomaly" title="Lonjakan Volume: ${volumeVsAvg.toFixed(0)}% vs Rata-rata">📊 Lonjakan Vol</span>`;
        }

        // Confidence score with visual indicator
        const confidence = alert.confidence_score || 100;
        let confidenceClass = 'confidence-low';
        let confidenceIcon = '⚪';
        if (confidence >= 85) {
            confidenceClass = 'confidence-extreme';
            confidenceIcon = '🔴';
        } else if (confidence >= 70) {
            confidenceClass = 'confidence-high';
            confidenceIcon = '🟠';
        } else if (confidence >= 50) {
            confidenceClass = 'confidence-medium';
            confidenceIcon = '🟡';
        }
        const confidenceLabel = `Yakin ${confidence.toFixed(0)}%`;

        // Enhanced message HTML
        const messageHtml = alert.message ?
            `<div style="font-size: 0.7rem; color: #555; margin-top: 4px; max-width: 200px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis;" title="${alert.message}">${alert.message}</div>` : '';

        // Alert type badge
        const alertType = alert.alert_type || 'SINGLE_TRADE';
        const alertTypeBadge = alertType !== 'SINGLE_TRADE' ?
            `<span style="font-size:0.65em; padding:2px 4px; background:#333; color:#fff; border-radius:3px; margin-left:4px;">${alertType}</span>` : '';

        // Render cell with event stopPropagation to prevent row click from overriding symbol click
        const symbolCellHtml = `
            <td data-label="Saham" class="col-symbol">
                <div style="display: flex; align-items: center; gap: 4px;">
                    <strong class="clickable-symbol" onclick="event.stopPropagation(); openCandleModal('${alert.stock_symbol}')">${alert.stock_symbol}</strong>
                    ${alertTypeBadge}
                </div>
                <span class="${confidenceClass}" style="font-size:0.7em;" title="Skor Keyakinan">${confidenceIcon} ${confidenceLabel}</span>
                ${messageHtml}
            </td>
        `;

        // Row Content with enhanced data
        row.innerHTML = `
            <td data-label="Waktu" class="col-time" title="${new Date(alert.detected_at).toLocaleString('id-ID')}">${formatTime(alert.detected_at)}</td>
            ${symbolCellHtml}
            <td data-label="Aksi"><span class="badge ${badgeClass}">${actionText}</span></td>
            <td data-label="Harga" class="col-price">${formatNumber(price)} ${priceDiff}</td>
            <td data-label="Nilai" class="text-right value-highlight" title="Total Nilai: Rp ${formatNumber(val)}">${formatCurrency(val)}</td>
            <td data-label="Volume" class="text-right" title="${formatNumber(volume)} lot">${formatNumber(volume)} Lot</td>
            <td data-label="Details">
                <div style="display: flex; flex-direction: column; gap: 2px;">
                    <span style="font-size:0.85em; color:var(--text-secondary);">${alert.market_board || 'RG'}</span>
                    ${anomalyHtml}
                    ${!anomalyHtml ? `<span style="font-size:0.75em; color:#aaa;">${alertType === 'ACCUMULATION' ? 'Akumulasi' : 'Transaksi Besar'}</span>` : ''}
                    ${zScore > 0 ? `<span style="font-size:0.7em; color:#888;" title="Statistical Anomaly Score">Z: ${zScore.toFixed(2)}</span>` : ''}
                    <span style="font-size: 0.65em; color: var(--accent-blue); margin-top: 2px;">Klik info ↗</span>
                </div>
            </td>
        `;
        tbody.appendChild(row);
    });
}

function updateStatsTicker() {
    if (!stats) return;

    // Use || 0 to handle cases where backend might return null or 0
    const totalTrades = stats.total_whale_trades || 0;
    const buyVol = stats.buy_volume_lots || 0;
    const sellVol = stats.sell_volume_lots || 0;
    const largestVal = stats.largest_trade_value || 0;

    document.getElementById('total-alerts').innerText = formatNumber(totalTrades);
    document.getElementById('total-volume').innerText = formatNumber(buyVol + sellVol) + " Lot";
    document.getElementById('largest-value').innerText = formatCurrency(largestVal);
}

// Initial Load & Event Listeners
document.addEventListener('DOMContentLoaded', () => {
    fetchAlerts(true); // true = reset mode
    fetchStats();

    // SSE Realtime Connection
    const evtSource = new EventSource('/api/events');

    evtSource.onmessage = function (event) {
        try {
            const msg = JSON.parse(event.data);
            if (msg.event === 'whale_alert') {
                const newAlert = msg.payload;
                // Prepend new alert
                alerts.unshift(newAlert);
                // Keep limit for performance
                if (alerts.length > CONFIG.MAX_ALERTS_CACHE) alerts.pop();
                currentOffset = Math.min(currentOffset + 1, CONFIG.MAX_ALERTS_CACHE); // Update offset

                renderAlerts();

                // Also refresh stats occasionally or increment locally
                // For simplicity, we just refetch stats on new alert
                fetchStats();
            }
        } catch (e) {
            console.error("SSE Parse Error", e);
        }
    };

    evtSource.onerror = function (err) {
        console.error("SSE Error:", err);
    };

    // Keep stats polling for aggregated values that might change from other sources
    setInterval(fetchStats, CONFIG.STATS_POLL_INTERVAL);

    document.getElementById('refresh-btn').addEventListener('click', () => {
        updateFilters();
        currentOffset = 0;
        hasMore = true;
        refreshAllData();

        // Visual feedback
        const btn = document.getElementById('refresh-btn');
        btn.style.transform = 'rotate(360deg)';
        setTimeout(() => btn.style.transform = '', 300);
    });

    // Search with debouncing to avoid too many requests
    let searchDebounceTimer = null;
    document.getElementById('search').addEventListener('input', (e) => {
        // Clear previous timer
        if (searchDebounceTimer) {
            clearTimeout(searchDebounceTimer);
        }

        // Visual feedback - show searching state
        const searchInput = e.target;
        searchInput.style.borderColor = '#ffd700';

        // Set new timer
        searchDebounceTimer = setTimeout(() => {
            updateFilters();
            currentOffset = 0;
            hasMore = true;
            refreshAllData();

            // Reset border color
            searchInput.style.borderColor = '';
        }, 500); // Wait 500ms after user stops typing
    });

    // Immediate filter for dropdowns
    document.getElementById('filter-action').addEventListener('change', () => {
        updateFilters();
        currentOffset = 0;
        hasMore = true;
        refreshAllData();

        // Visual feedback
        highlightActiveFilters();
    });

    document.getElementById('filter-amount').addEventListener('change', () => {
        updateFilters();
        currentOffset = 0;
        hasMore = true;
        refreshAllData();

        // Visual feedback
        highlightActiveFilters();
    });

    document.getElementById('filter-board').addEventListener('change', () => {
        updateFilters();
        currentOffset = 0;
        hasMore = true;
        refreshAllData();

        // Visual feedback
        highlightActiveFilters();
    });

    // Clear all filters button
    document.getElementById('clear-filters-btn').addEventListener('click', () => {
        // Reset all filter inputs
        document.getElementById('search').value = '';
        document.getElementById('filter-action').value = 'ALL';
        document.getElementById('filter-amount').value = '0';
        document.getElementById('filter-board').value = 'ALL';

        // Update filters and fetch
        updateFilters();
        currentOffset = 0;
        hasMore = true;
        refreshAllData();

        // Visual feedback
        const btn = document.getElementById('clear-filters-btn');
        btn.style.transform = 'scale(0.95)';
        setTimeout(() => btn.style.transform = '', 200);
    });


    // Infinite scroll: detect when user scrolls near bottom
    const whaleTableContainer = document.querySelector('.whale-alerts-section .table-container');
    if (whaleTableContainer) {
        whaleTableContainer.addEventListener('scroll', () => {
            const { scrollTop, scrollHeight, clientHeight } = whaleTableContainer;
            // Trigger load more when scroll within threshold of bottom
            if (scrollHeight - scrollTop - clientHeight < CONFIG.SCROLL_THRESHOLD && hasMore && !isLoading) {
                fetchAlerts(false); // false = append mode
            }
        });
    }

    // Pattern Analysis Functionality
    setupPatternAnalysis();

    // Fetch Accumulation/Distribution Summary
    fetchAccumulationSummary();

    // ===== HELP MODAL =====
    const helpBtn = document.getElementById('help-btn');
    const modal = document.getElementById('help-modal');
    const modalClose = document.getElementById('modal-close');
    const modalGotIt = document.getElementById('modal-got-it');

    if (helpBtn && modal) {
        // Open modal
        helpBtn.addEventListener('click', () => {
            modal.classList.add('show');
        });

        // Close modal handlers
        const closeModal = () => {
            modal.classList.remove('show');
        };

        if (modalClose) {
            modalClose.addEventListener('click', closeModal);
        }

        if (modalGotIt) {
            modalGotIt.addEventListener('click', closeModal);
        }

        // Close on outside click
        modal.addEventListener('click', (e) => {
            if (e.target === modal) {
                closeModal();
            }
        });

        // Close on ESC key
        document.addEventListener('keydown', (e) => {
            if (e.key === 'Escape' && modal.classList.contains('show')) {
                closeModal();
            }
        });
    }

    // Phase 4: UI Integration - Initialization
    setupAnalyticsTabs();
    setupCandleModal();
    setupFollowupModal();
    fetchMarketIntelligence();
    fetchAnalyticsHubData();
    fetchGlobalPerformance();

    setInterval(() => {
        fetchMarketIntelligence();
        fetchAnalyticsHubData();
        fetchOrderFlow();
    }, CONFIG.ANALYTICS_POLL_INTERVAL);
});

// ===== ACCUMULATION/DISTRIBUTION SUMMARY =====
async function fetchAccumulationSummary() {
    const summaryLoading = document.getElementById('summary-loading');

    if (summaryLoading) summaryLoading.style.display = 'block';

    try {
        const res = await fetch(`${API_BASE}/accumulation-summary`);
        const data = await res.json();

        const accumulation = data.accumulation || [];
        const distribution = data.distribution || [];

        // Update counters
        const accCount = document.getElementById('accumulation-count');
        const distCount = document.getElementById('distribution-count');
        if (accCount) accCount.textContent = accumulation.length;
        if (distCount) distCount.textContent = distribution.length;

        // Render both tables
        renderTable('accumulation', accumulation);
        renderTable('distribution', distribution);
    } catch (err) {
        console.error("Failed to fetch accumulation summary:", err);
    } finally {
        if (summaryLoading) summaryLoading.style.display = 'none';
    }
}

function renderTable(type, data) {
    const tbody = document.getElementById(`${type}-table-body`);
    const placeholder = document.getElementById(`${type}-placeholder`);

    if (!tbody) return;

    tbody.innerHTML = '';

    if (data.length === 0) {
        if (placeholder) placeholder.style.display = 'block';
        return;
    }

    if (placeholder) placeholder.style.display = 'none';

    data.forEach(item => {
        const row = document.createElement('tr');

        // Net value color
        const netValueClass = item.net_value >= 0 ? 'diff-positive' : 'diff-negative';
        const netValueSign = item.net_value >= 0 ? '+' : '';

        row.innerHTML = `
            <td data-label="Saham" class="col-symbol">${item.stock_symbol}</td>
            <td data-label="BUY %" class="text-right">
                <span class="diff-positive" style="font-weight: 600;">${item.buy_percentage.toFixed(1)}%</span>
            </td>
            <td data-label="SELL %" class="text-right">
                <span class="diff-negative" style="font-weight: 600;">${item.sell_percentage.toFixed(1)}%</span>
            </td>
            <td data-label="Net Value" class="text-right">
                <span class="${netValueClass}" style="font-weight: 600;">${netValueSign}${formatCurrency(Math.abs(item.net_value))}</span>
            </td>
            <td data-label="Alerts" class="text-right">${item.total_count}</td>
            <td data-label="Total Value" class="text-right value-highlight">${formatCurrency(item.total_value)}</td>
        `;

        tbody.appendChild(row);
    });
}

// ===== PATTERN ANALYSIS =====
let currentPatternType = 'accumulation';
let streamEventSource = null;

function setupPatternAnalysis() {
    const tabs = document.querySelectorAll('.tab-btn');
    const startBtn = document.getElementById('start-analysis-btn');
    const stopBtn = document.getElementById('stop-analysis-btn');
    const outputDiv = document.getElementById('llm-stream-output');
    const statusBadge = document.getElementById('llm-status');
    const symbolInputContainer = document.getElementById('symbol-input-container');

    // Tab switching
    tabs.forEach(tab => {
        tab.addEventListener('click', () => {
            // Stop any active stream
            stopStreamAnalysis();

            // Update active tab
            tabs.forEach(t => t.classList.remove('active'));
            tab.classList.add('active');

            // Update current type
            currentPatternType = tab.dataset.type;

            // Show/hide symbol input based on tab
            if (currentPatternType === 'symbol') {
                symbolInputContainer.style.display = 'block';
            } else {
                symbolInputContainer.style.display = 'none';
            }

            // Reset output
            resetOutput();
        });
    });

    // Start analysis
    startBtn.addEventListener('click', () => {
        startPatternAnalysis(currentPatternType);
    });

    // Stop analysis
    stopBtn.addEventListener('click', () => {
        stopStreamAnalysis();
    });
}

function startPatternAnalysis(type) {
    const outputDiv = document.getElementById('llm-stream-output');
    const statusBadge = document.getElementById('llm-status');
    const startBtn = document.getElementById('start-analysis-btn');
    const stopBtn = document.getElementById('stop-analysis-btn');

    // For symbol type, validate input
    let url = `/api/patterns/${type}/stream`;
    if (type === 'symbol') {
        const symbolInput = document.getElementById('symbol-input');
        const symbol = symbolInput.value.trim().toUpperCase();

        if (!symbol || symbol.length < 1) {
            outputDiv.innerHTML = `
                <div class="placeholder">
                    <span class="placeholder-icon">⚠️</span>
                    <p style="color: var(--accent-sell);">Silakan masukkan kode saham terlebih dahulu</p>
                </div>
            `;
            return;
        }

        url = `/api/patterns/symbol/stream?symbol=${symbol}&limit=20`;
    }

    // Update UI
    startBtn.style.display = 'none';
    stopBtn.style.display = 'flex';
    statusBadge.textContent = 'Streaming...';
    statusBadge.className = 'status-badge streaming';

    // Clear output and show loading
    outputDiv.innerHTML = '<div class="stream-loading">🤖 Menganalisis data...</div>';

    // Start SSE connection
    streamEventSource = new EventSource(url);

    let streamText = '';

    streamEventSource.onmessage = (event) => {
        const chunk = event.data;

        // Remove loading message on first chunk
        if (streamText === '') {
            outputDiv.innerHTML = '<div class="message-bubble"><div class="streaming-text"></div></div>';
        }

        streamText += chunk;

        // Parse markdown
        const htmlContent = marked.parse(streamText);

        // Update output
        const textContainer = outputDiv.querySelector('.streaming-text');
        if (textContainer) {
            textContainer.innerHTML = `${htmlContent}<span class="streaming-cursor"></span>`;
        }

        // Auto-scroll to bottom
        outputDiv.scrollTop = outputDiv.scrollHeight;
    };

    streamEventSource.addEventListener('done', () => {
        // Remove cursor and update status
        const htmlContent = marked.parse(streamText);
        outputDiv.innerHTML = `<div class="message-bubble"><div class="streaming-text">${htmlContent}</div></div>`;

        statusBadge.textContent = 'Completed';
        statusBadge.className = 'status-badge';
        streamEventSource.close();
        streamEventSource = null;

        // Reset buttons
        startBtn.style.display = 'flex';
        stopBtn.style.display = 'none';
    });

    streamEventSource.addEventListener('error', (event) => {
        console.error('SSE Error:', event);

        // Handle error
        if (streamText === '') {
            outputDiv.innerHTML = `
                <div class="placeholder">
                    <span class="placeholder-icon">⚠️</span>
                    <p style="color: var(--accent-sell);">Error: Tidak dapat memuat analisis. Pastikan LLM diaktifkan dan data tersedia.</p>
                </div>
            `;
        } else {
            // Stream was interrupted
            outputDiv.innerHTML = `<div class="streaming-text">${streamText}</div>`;
        }

        statusBadge.textContent = 'Error';
        statusBadge.className = 'status-badge error';

        streamEventSource.close();
        streamEventSource = null;

        // Reset buttons
        startBtn.style.display = 'flex';
        stopBtn.style.display = 'none';
    });
}

function stopStreamAnalysis() {
    if (streamEventSource) {
        streamEventSource.close();
        streamEventSource = null;

        const statusBadge = document.getElementById('llm-status');
        const startBtn = document.getElementById('start-analysis-btn');
        const stopBtn = document.getElementById('stop-analysis-btn');

        statusBadge.textContent = 'Stopped';
        statusBadge.className = 'status-badge';

        startBtn.style.display = 'flex';
        stopBtn.style.display = 'none';
    }
}

function resetOutput() {
    const outputDiv = document.getElementById('llm-stream-output');
    const statusBadge = document.getElementById('llm-status');

    outputDiv.innerHTML = `
        <div class="placeholder">
            <span class="placeholder-icon">🧠</span>
            <p>Click "Start Analysis" untuk memulai analisis AI real-time</p>
        </div>
    `;

    statusBadge.textContent = 'Ready';
    statusBadge.className = 'status-badge';
}

function updateFilters() {
    currentFilters.search = document.getElementById('search').value.trim().toUpperCase();
    currentFilters.action = document.getElementById('filter-action').value;
    currentFilters.amount = parseFloat(document.getElementById('filter-amount').value) || 0;
    currentFilters.board = document.getElementById('filter-board').value;

    // Update UI to show active filters
    highlightActiveFilters();
}

function highlightActiveFilters() {
    // Highlight search if active
    const searchInput = document.getElementById('search');
    if (currentFilters.search) {
        searchInput.style.backgroundColor = 'rgba(14, 203, 129, 0.1)';
        searchInput.style.borderColor = 'var(--accent-buy)';
    } else {
        searchInput.style.backgroundColor = '';
        searchInput.style.borderColor = '';
    }

    // Highlight action filter if not ALL
    const actionSelect = document.getElementById('filter-action');
    if (currentFilters.action !== 'ALL') {
        actionSelect.style.backgroundColor = 'rgba(14, 203, 129, 0.1)';
        actionSelect.style.borderColor = 'var(--accent-buy)';
    } else {
        actionSelect.style.backgroundColor = '';
        actionSelect.style.borderColor = '';
    }

    // Highlight amount filter if active
    const amountSelect = document.getElementById('filter-amount');
    if (currentFilters.amount > 0) {
        amountSelect.style.backgroundColor = 'rgba(14, 203, 129, 0.1)';
        amountSelect.style.borderColor = 'var(--accent-buy)';
    } else {
        amountSelect.style.backgroundColor = '';
        amountSelect.style.borderColor = '';
    }

    // Highlight board filter if not ALL
    const boardSelect = document.getElementById('filter-board');
    if (currentFilters.board !== 'ALL') {
        boardSelect.style.backgroundColor = 'rgba(14, 203, 129, 0.1)';
        boardSelect.style.borderColor = 'var(--accent-buy)';
    } else {
        boardSelect.style.backgroundColor = '';
        boardSelect.style.borderColor = '';
    }

    // Show active filter count
    updateActiveFilterCount();
}

function updateActiveFilterCount() {
    let activeCount = 0;
    if (currentFilters.search) activeCount++;
    if (currentFilters.action !== 'ALL') activeCount++;
    if (currentFilters.amount > 0) activeCount++;
    if (currentFilters.board !== 'ALL') activeCount++;

    // Show/hide clear filters button
    const clearBtn = document.getElementById('clear-filters-btn');
    if (clearBtn) {
        if (activeCount > 0) {
            clearBtn.style.display = 'flex';
        } else {
            clearBtn.style.display = 'none';
        }
    }

    // Update section title to show active filter count
    const sectionTitle = document.querySelector('.section-title-small h3');
    if (sectionTitle) {
        if (activeCount > 0) {
            sectionTitle.innerHTML = `Pencarian & Filter <span style="background: var(--accent-buy); color: #000; padding: 2px 8px; border-radius: 12px; font-size: 0.7em; margin-left: 8px;">${activeCount}</span>`;
        } else {
            sectionTitle.textContent = 'Pencarian & Filter';
        }
    }
}
// ===== MARKET INTELLIGENCE =====
async function fetchMarketIntelligence() {
    const symbol = currentFilters.search;
    if (!symbol) {
        document.getElementById('market-intelligence').style.opacity = '0.5';
        return;
    }
    document.getElementById('market-intelligence').style.opacity = '1';

    try {
        // 1. Fetch Regime
        const regimeRes = await fetch(`${API_BASE}/regimes?symbol=${symbol}`);
        const regimeData = await regimeRes.json();
        updateRegimeUI(regimeData);

        // 2. Fetch Baseline
        const baselineRes = await fetch(`${API_BASE}/baselines?symbol=${symbol}`);
        const baselineData = await baselineRes.json();
        updateBaselineUI(baselineData);

        // 3. Fetch Patterns
        const patternRes = await fetch(`${API_BASE}/patterns?symbol=${symbol}&since=${new Date(Date.now() - 24 * 3600 * 1000).toISOString()}`);
        const patternData = await patternRes.json();
        updatePatternsUI(patternData.patterns || []);

        // 4. Fetch Order Flow
        fetchOrderFlow();

    } catch (err) {
        console.error("Failed to fetch market intel:", err);
    }
}

function updateRegimeUI(data) {
    const regimeEl = document.getElementById('intel-regime');
    const descEl = document.getElementById('intel-regime-desc');
    const badge = document.getElementById('market-regime');

    if (!data || !data.regime) {
        regimeEl.textContent = 'UNKNOWN';
        descEl.textContent = 'No regime data available';
        badge.style.display = 'none';
        return;
    }

    const regime = data.regime.replace(/_/g, ' ');
    regimeEl.textContent = regime;
    descEl.textContent = `Tingkat Keyakinan: ${formatPercent(data.confidence * 100)} | Update terakhir: ${formatTime(data.detected_at)}`;

    // Update Header Badge
    badge.textContent = regime;
    badge.style.display = 'inline-block';

    // Reset classes
    badge.className = 'status-badge';
    const lowRegime = data.regime.toLowerCase();
    if (lowRegime.includes('up')) badge.classList.add('regime-trending-up');
    else if (lowRegime.includes('down')) badge.classList.add('regime-trending-down');
    else if (lowRegime.includes('ranging')) badge.classList.add('regime-ranging');
    else if (lowRegime.includes('volatile')) badge.classList.add('regime-volatile');
}

function updateBaselineUI(data) {
    if (!data) return;
    document.getElementById('b-avg-vol').textContent = formatNumber(data.mean_volume_lots) + ' Lot';
    document.getElementById('b-std-dev').textContent = formatNumber(data.std_dev_price ? data.std_dev_price.toFixed(2) : 0);
}

function updatePatternsUI(patterns) {
    const list = document.getElementById('pattern-list');
    list.innerHTML = '';

    if (patterns.length === 0) {
        list.innerHTML = '<div class="placeholder-small">Tidak ada pola yang terdeteksi baru-baru ini</div>';
        return;
    }

    patterns.forEach(p => {
        const div = document.createElement('div');
        div.className = 'pattern-item';
        div.innerHTML = `
            <span class="pattern-type">${p.pattern_type.replace(/_/g, ' ')}</span>
            <span class="pattern-conf">Yakin ${formatPercent(p.confidence * 100)}</span>
        `;
        list.appendChild(div);
    });
}

// ===== ANALYTICS HUB =====
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
            document.getElementById(target).classList.add('active');
        });
    });
}

async function fetchAnalyticsHubData() {
    const symbol = currentFilters.search;

    // Correlations
    if (symbol) {
        try {
            const res = await fetch(`${API_BASE}/analytics/correlations?symbol=${symbol}`);
            const data = await res.json();
            renderCorrelations(data.correlations || []);
        } catch (err) {
            console.error("Correlations fetch failed", err);
        }
    }

    // Daily Performance
    try {
        const params = new URLSearchParams();
        params.append('limit', '10');
        if (symbol) params.append('symbol', symbol);

        const res = await fetch(`${API_BASE}/analytics/performance/daily?${params.toString()}`);
        const data = await res.json();
        renderDailyPerformance(data.performance || []);
    } catch (err) {
        console.error("Performance fetch failed", err);
    }
}

function renderCorrelations(correlations) {
    const container = document.getElementById('correlation-container');
    container.innerHTML = '';

    if (correlations.length === 0) {
        container.innerHTML = '<div class="placeholder-small">No similar stocks found</div>';
        return;
    }

    correlations.forEach(c => {
        const other = c.stock_a === currentFilters.search ? c.stock_b : c.stock_a;
        const coef = c.correlation_coefficient;
        let colorClass = 'corr-low';
        if (coef > 0.7) colorClass = 'corr-high';
        else if (coef > 0.4) colorClass = 'corr-med';

        const div = document.createElement('div');
        div.className = 'corr-item';
        div.innerHTML = `
            <span class="corr-symbol">${other}</span>
            <span class="corr-value ${colorClass}">${coef.toFixed(2)}</span>
        `;
        container.appendChild(div);
    });
}

function renderDailyPerformance(performance) {
    const tbody = document.getElementById('daily-performance-body');
    if (!tbody) return;
    tbody.innerHTML = '';

    if (!performance || performance.length === 0) {
        tbody.innerHTML = '<tr><td colspan="4" class="text-center">Belum ada data performa tercatat</td></tr>';
        return;
    }

    performance.forEach(p => {
        // Safe access for daily stats
        const signals = p.total_signals || 0;
        const wins = p.wins || 0;
        const winRate = signals > 0 ? (wins / signals) * 100 : 0;
        const profit = p.total_profit_pct || 0;
        const strategyName = (p.strategy || 'Unknown').replace(/_/g, ' ');

        const row = document.createElement('tr');
        row.innerHTML = `
            <td>${p.day ? new Date(p.day).toLocaleDateString('id-ID', { day: '2-digit', month: 'short' }) : '-'}</td>
            <td>${strategyName}</td>
            <td class="text-right ${winRate >= 50 ? 'diff-positive' : 'diff-negative'}">${winRate.toFixed(1)}%</td>
            <td class="text-right ${profit >= 0 ? 'diff-positive' : 'diff-negative'}">${profit.toFixed(2)}%</td>
        `;
        tbody.appendChild(row);
    });
}

async function fetchGlobalPerformance() {
    try {
        const res = await fetch(`${API_BASE}/signals/performance?strategy=ALL`);
        const data = await res.json();
        if (data && data.win_rate !== undefined) {
            document.getElementById('global-win-rate').textContent = (data.win_rate * 100).toFixed(1) + '%';
        }
    } catch (err) {
        console.error("Global performance fetch failed", err);
    }
}

// ===== ORDER FLOW =====
async function fetchOrderFlow() {
    const symbol = currentFilters.search;
    if (!symbol) return;

    try {
        const res = await fetch(`${API_BASE}/orderflow?symbol=${symbol}&limit=50`);
        const data = await res.json();

        if (data && data.flows && data.flows.length > 0) {
            let totalBuy = 0;
            let totalSell = 0;

            data.flows.forEach(item => {
                totalBuy += item.buy_volume;
                totalSell += item.sell_volume;
            });

            const total = totalBuy + totalSell;
            const buyPct = (totalBuy / total) * 100;
            const sellPct = (totalSell / total) * 100;

            document.getElementById('buy-pressure-fill').style.width = `${buyPct}%`;
            document.getElementById('sell-pressure-fill').style.width = `${sellPct}%`;
            document.getElementById('buy-pressure-pct').textContent = `${buyPct.toFixed(1)}% BELI`;
            document.getElementById('sell-pressure-pct').textContent = `${sellPct.toFixed(1)}% JUAL`;
        }
    } catch (err) {
        console.error("Order flow fetch failed", err);
    }
}

// ===== WHALE FOLLOWUP =====
function setupFollowupModal() {
    const modal = document.getElementById('followup-modal');
    const closeBtn = document.getElementById('followup-modal-close');
    if (closeBtn) closeBtn.onclick = () => modal.classList.remove('show');
}

async function openFollowupModal(alertId, symbol, alertPrice) {
    const modal = document.getElementById('followup-modal');
    const loading = document.getElementById('followup-loading');
    const content = document.getElementById('followup-content');

    document.getElementById('followup-title').textContent = `🐳 Followup: ${symbol}`;
    loading.style.display = 'block';
    content.style.display = 'none';
    modal.classList.add('show');

    try {
        const res = await fetch(`${API_BASE}/whales/${alertId}/followup`);
        const data = await res.json();

        loading.style.display = 'none';
        content.style.display = 'block';

        const currentPrice = data.current_price;
        const changePct = ((currentPrice - alertPrice) / alertPrice) * 100;
        const sign = changePct >= 0 ? '+' : '';
        const color = changePct >= 0 ? 'var(--accent-buy)' : 'var(--accent-sell)';

        document.getElementById('f-alert-price').textContent = formatNumber(alertPrice);
        document.getElementById('f-current-price').textContent = formatNumber(currentPrice);
        document.getElementById('f-price-change').textContent = `${sign}${changePct.toFixed(2)}%`;
        document.getElementById('f-price-change').style.color = color;
        document.getElementById('f-time-elapsed').textContent = formatTime(data.detected_at);

        const analysis = document.getElementById('followup-analysis');
        if (changePct > 2) {
            analysis.innerHTML = `🌟 <strong>Kenaikan Berlanjut!</strong> Deteksi bandar terbukti efektif. Harga telah naik signifikan sejak deteksi awal.`;
            analysis.style.borderColor = 'var(--accent-buy)';
            analysis.style.background = 'rgba(14, 203, 129, 0.1)';
        } else if (changePct < -2) {
            analysis.innerHTML = `⚠️ <strong>Koreksi Terdeteksi.</strong> Harga turun dari level deteksi bandar. Kemungkinan terjadi aksi jual atau ambil untung.`;
            analysis.style.borderColor = 'var(--accent-sell)';
            analysis.style.background = 'rgba(246, 70, 93, 0.1)';
        } else {
            analysis.innerHTML = `⚖️ <strong>Konsolidasi.</strong> Harga masih berada di sekitar level deteksi bandar. Menunggu konfirmasi arah selanjutnya.`;
            analysis.style.borderColor = 'var(--accent-blue)';
            analysis.style.background = 'rgba(59, 130, 246, 0.1)';
        }

    } catch (err) {
        console.error("Followup fetch failed", err);
        loading.textContent = "Gagal memuat analisis followup.";
    }
}

// ===== CANDLE VIEWER MODAL =====
let currentCandleSymbol = '';
let currentCandleTimeframe = '1h';

function setupCandleModal() {
    const modal = document.getElementById('candle-modal');
    const closeBtn = document.getElementById('candle-modal-close');
    const tabs = document.querySelectorAll('.c-tab');

    if (closeBtn) closeBtn.onclick = () => modal.classList.remove('show');

    tabs.forEach(tab => {
        tab.onclick = () => {
            tabs.forEach(t => t.classList.remove('active'));
            tab.classList.add('active');
            currentCandleTimeframe = tab.dataset.timeframe;
            fetchCandleData(currentCandleSymbol, currentCandleTimeframe);
        };
    });

    // Close on outside click
    window.addEventListener('click', (e) => {
        if (e.target === modal) modal.classList.remove('show');
    });
}

function openCandleModal(symbol) {
    currentCandleSymbol = symbol;
    const modal = document.getElementById('candle-modal');
    document.getElementById('candle-modal-title').textContent = `📉 Market Details: ${symbol}`;

    // Reset tabs to default (1h)
    document.querySelectorAll('.c-tab').forEach(t => {
        t.classList.remove('active');
        if (t.dataset.timeframe === '1h') t.classList.add('active');
    });
    currentCandleTimeframe = '1h';

    modal.classList.add('show');
    fetchCandleData(symbol, '1h');
}

async function fetchCandleData(symbol, timeframe) {
    const tbody = document.getElementById('candle-list-body');
    if (tbody) tbody.innerHTML = '<tr><td colspan="6" class="text-center">Memuat data histori harga...</td></tr>';

    try {
        const res = await fetch(`${API_BASE}/candles?symbol=${symbol}&timeframe=${timeframe}&limit=50`);
        const data = await res.json();
        renderCandles(data.candles || []);
    } catch (err) {
        console.error("Candle fetch failed", err);
        if (tbody) tbody.innerHTML = '<tr><td colspan="6" class="text-center">Gagal memuat data histori</td></tr>';
    }
}

function renderCandles(candles) {
    const tbody = document.getElementById('candle-list-body');
    if (!tbody) return;
    tbody.innerHTML = '';

    if (candles.length === 0) {
        tbody.innerHTML = '<tr><td colspan="6" class="text-center">Tidak ada data untuk rentang waktu ini</td></tr>';
        return;
    }

    candles.forEach(c => {
        const row = document.createElement('tr');
        const time = new Date(c.time).toLocaleString('id-ID', {
            day: '2-digit', month: 'short', hour: '2-digit', minute: '2-digit'
        });

        row.innerHTML = `
            <td>${time}</td>
            <td class="text-right">${formatNumber(c.open)}</td>
            <td class="text-right">${formatNumber(c.high)}</td>
            <td class="text-right">${formatNumber(c.low)}</td>
            <td class="text-right ${c.close >= c.open ? 'diff-positive' : 'diff-negative'}">${formatNumber(c.close)}</td>
            <td class="text-right">${formatNumber(c.volume)}</td>
        `;
        tbody.appendChild(row);
    });
}

// Global expose for onclick
window.openCandleModal = openCandleModal;
