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
  TIME_HOUR: 3600
};

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
    
    if (diffSec < CONFIG.TIME_MINUTE) return `${diffSec}s ago`;
    if (diffSec < CONFIG.TIME_HOUR) return `${Math.floor(diffSec / CONFIG.TIME_MINUTE)}m ago`;
    
    return date.toLocaleTimeString('id-ID', { hour: '2-digit', minute: '2-digit' });
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
    
    // Send all filters to backend (all now supported!)
    if (currentFilters.search) {
        params.append('symbol', currentFilters.search);
    }
    if (currentFilters.action !== 'ALL') {
        params.append('type', currentFilters.action);
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

async function fetchStats() {
    try {
        const res = await fetch(`${API_BASE}/whales/stats`);
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
        if (loadingDiv) loadingDiv.innerText = 'No alerts found matching filters.';
        if (loadingDiv) loadingDiv.style.display = 'block';
        return;
    } else {
        if (loadingDiv) loadingDiv.style.display = 'none';
    }

    filtered.forEach(alert => {
        const row = document.createElement('tr');
        
        let badgeClass = 'unknown';
        if (alert.Action === 'BUY') badgeClass = 'buy';
        if (alert.Action === 'SELL') badgeClass = 'sell';

        // Mapped fields from WebhookPayload
        const price = alert.Price || alert.TriggerPrice; // Support both just in case
        const volume = alert.VolumeLots || alert.TriggerVolumeLots;
        const val = alert.TotalValue || alert.TriggerValue;

        const avgPrice = alert.AvgPrice || 0;
        let priceDiff = '';
        if (avgPrice > 0) {
            const pct = ((price - avgPrice) / avgPrice) * 100;
            const sign = pct >= 0 ? '+' : '';
            const type = pct >= 0 ? 'diff-positive' : 'diff-negative';
            priceDiff = `<span class="${type}">(${sign}${pct.toFixed(1)}%)</span>`;
        }

        const zScore = (alert.Metadata && alert.Metadata.z_score) ? alert.Metadata.z_score : (alert.ZScore || 0);
        const anomalyHtml = zScore >= 3.0 ? 
            `<span class="table-anomaly" title="Z-Score: ${zScore.toFixed(2)}">⚠️ Anomaly</span>` : '';

        // Generate message HTML only if message exists
        const messageHtml = alert.Message ? 
            `<div style="font-size: 0.7rem; color: #555; margin-top: 4px; max-width: 200px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis;" title="${alert.Message}">${alert.Message}</div>` : '';

        // Row Content
        row.innerHTML = `
            <td data-label="Time" class="col-time">${formatTime(alert.DetectedAt)}</td>
            <td data-label="Symbol" class="col-symbol">
                ${alert.StockSymbol} 
                <span style="font-size:0.7em; color:#666;">${(alert.ConfidenceScore||100).toFixed(0)}%</span>
                ${messageHtml}
            </td>
            <td data-label="Action"><span class="badge ${badgeClass}">${alert.Action}</span></td>
            <td data-label="Price" class="col-price">${formatNumber(price)} ${priceDiff}</td>
            <td data-label="Value" class="text-right value-highlight">${formatCurrency(val)}</td>
            <td data-label="Volume" class="text-right">${formatNumber(volume)} Lots</td>
            <td data-label="Details">
                <span style="font-size:0.85em; color:var(--text-secondary);">${alert.MarketBoard}</span>
                ${anomalyHtml}
            </td>
        `;
        tbody.appendChild(row);
    });
}

function updateStatsTicker() {
    if (stats.total_whale_trades) {
        document.getElementById('total-alerts').innerText = formatNumber(stats.total_whale_trades);
        document.getElementById('total-volume').innerText = formatNumber(stats.buy_volume_lots + stats.sell_volume_lots) + " Lots";
        document.getElementById('largest-value').innerText = formatCurrency(stats.largest_trade_value);
    }
}

// Initial Load & Event Listeners
document.addEventListener('DOMContentLoaded', () => {
    fetchAlerts(true); // true = reset mode
    fetchStats();

    // SSE Realtime Connection
    const evtSource = new EventSource('/api/events');
    
    evtSource.onmessage = function(event) {
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
    
    evtSource.onerror = function(err) {
        console.error("SSE Error:", err);
    };

    // Keep stats polling for aggregated values that might change from other sources
    setInterval(fetchStats, CONFIG.STATS_POLL_INTERVAL); 

    document.getElementById('refresh-btn').addEventListener('click', () => {
        updateFilters();
        currentOffset = 0;
        hasMore = true;
        fetchAlerts(true);
        fetchStats();
    });

    // All filters now trigger server refetch
    document.getElementById('search').addEventListener('input', () => {
        updateFilters();
        currentOffset = 0;
        hasMore = true;
        fetchAlerts(true);
    });
    
    document.getElementById('filter-action').addEventListener('change', () => {
        updateFilters();
        currentOffset = 0;
        hasMore = true;
        fetchAlerts(true);
    });
    
    document.getElementById('filter-amount').addEventListener('change', () => {
        updateFilters();
        currentOffset = 0;
        hasMore = true;
        fetchAlerts(true);
    });
    
    document.getElementById('filter-board').addEventListener('change', () => {
        updateFilters();
        currentOffset = 0;
        hasMore = true;
        fetchAlerts(true);
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
});

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
            outputDiv.innerHTML = '<div class="streaming-text"></div>';
        }

        streamText += chunk;
        
        // Parse markdown using marked.js
        const htmlContent = marked.parse(streamText);

        // Update output with streaming cursor
        outputDiv.innerHTML = `<div class="streaming-text">${htmlContent}<span class="streaming-cursor"></span></div>`;
        
        // Auto-scroll to bottom
        outputDiv.scrollTop = outputDiv.scrollHeight;
    };

    streamEventSource.addEventListener('done', () => {
        // Remove cursor and update status
        outputDiv.innerHTML = `<div class="streaming-text">${marked.parse(streamText)}</div>`;
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
    currentFilters.search = document.getElementById('search').value.toUpperCase();
    currentFilters.action = document.getElementById('filter-action').value;
    currentFilters.amount = parseFloat(document.getElementById('filter-amount').value);
    currentFilters.board = document.getElementById('filter-board').value;
}
