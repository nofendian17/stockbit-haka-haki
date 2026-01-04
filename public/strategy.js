// ===== CONFIGURATION =====
const STRATEGY_CONFIG = {
    MAX_VISIBLE_SIGNALS: 100,
    ANIMATION_DELAY: 10,
    TRANSITION_DURATION: 300,
    LOOKBACK_MINUTES: 60
};

// ===== STATE =====
let strategyEventSource = null;
let activeStrategyFilter = 'ALL';
let renderedSignalIds = new Set();

// ===== UTILITY FUNCTIONS =====
/**
 * Safely get DOM element with error logging
 * @param {string} id - Element ID
 * @param {string} context - Context for error message
 * @returns {HTMLElement|null}
 */
function safeGetElement(id, context = 'Strategy') {
    const element = document.getElementById(id);
    if (!element) {
        console.error(`[${context}] Element not found: ${id}`);
    }
    return element;
}

// ===== INITIALIZATION =====
document.addEventListener('DOMContentLoaded', () => {
    initStrategySystem();
});

function initStrategySystem() {
    // Verify critical elements exist before initializing
    const tbody = safeGetElement('signals-table-body', 'Init');
    if (!tbody) {
        console.error('Critical element missing: signals-table-body not found in DOM');
        return;
    }

    setupStrategyTabs();

    // Fetch initial signals first, then connect SSE
    fetchInitialSignals().then(() => {
        connectStrategySSE();
    });
}

// ===== API FUNCTIONS =====
async function fetchInitialSignals() {
    const tbody = safeGetElement('signals-table-body', 'FetchSignals');
    const placeholder = safeGetElement('signals-placeholder', 'FetchSignals');
    const loading = safeGetElement('signals-loading', 'FetchSignals');

    // Check if tbody exists
    if (!tbody) {
        return;
    }

    if (placeholder) placeholder.style.display = 'none';
    if (loading) loading.style.display = 'flex';

    try {
        let url = `/api/strategies/signals?lookback=${STRATEGY_CONFIG.LOOKBACK_MINUTES}`;
        if (activeStrategyFilter !== 'ALL') {
            url += `&strategy=${activeStrategyFilter}`;
        }

        const res = await fetch(url);
        const data = await res.json();

        if (loading) loading.style.display = 'none';

        if (data.signals && data.signals.length > 0) {
            // Sort by timestamp descending (newest first)
            const signals = data.signals.sort((a, b) => new Date(b.timestamp) - new Date(a.timestamp));

            signals.forEach(signal => {
                renderSignalRow(signal, true); // true = initial load, append to end
            });
        } else {
            if (placeholder && tbody.children.length === 0) {
                placeholder.style.display = 'flex';
            }
        }
    } catch (err) {
        console.error("Failed to fetch initial signals:", err);
        if (loading) loading.style.display = 'none';
        if (placeholder) placeholder.style.display = 'flex';
    }
}

// ===== UI SETUP FUNCTIONS =====
function setupStrategyTabs() {
    const tabs = document.querySelectorAll('.strategy-tab');

    tabs.forEach(tab => {
        tab.addEventListener('click', () => {
            // Remove active from all tabs
            tabs.forEach(t => t.classList.remove('active'));

            // Add active to clicked tab
            tab.classList.add('active');

            // Update filter
            activeStrategyFilter = tab.dataset.strategy;

            // Clear table and reconnect
            const tbody = safeGetElement('signals-table-body', 'TabSwitch');
            if (tbody) {
                tbody.innerHTML = '';
            }
            renderedSignalIds.clear();

            // Fetch initial signals first, then connect SSE
            // This prevents race condition where SSE signals arrive before initial load completes
            fetchInitialSignals().then(() => {
                connectStrategySSE();
            });
        });
    });
}

// ===== SSE CONNECTION =====
function connectStrategySSE() {
    if (strategyEventSource) {
        strategyEventSource.close();
    }

    const statusEl = safeGetElement('strategy-connection-status', 'SSE');
    const indicatorEl = safeGetElement('strategy-live-indicator', 'SSE');

    if (!statusEl || !indicatorEl) return;

    statusEl.textContent = 'Connecting...';
    indicatorEl.style.backgroundColor = '#FFD700';
    indicatorEl.style.animation = 'pulse 2s infinite';

    let url = '/api/strategies/signals/stream';
    if (activeStrategyFilter !== 'ALL') {
        url += `?strategy=${activeStrategyFilter}`;
    }

    strategyEventSource = new EventSource(url);

    strategyEventSource.addEventListener('connected', (e) => {
        statusEl.textContent = 'Live';
        indicatorEl.style.backgroundColor = '#0ECB81';
        indicatorEl.style.animation = 'none';

        const placeholder = safeGetElement('signals-placeholder', 'SSE');
        if (placeholder) placeholder.style.display = 'none';
    });

    strategyEventSource.addEventListener('signal', (e) => {
        try {
            const signal = JSON.parse(e.data);
            renderSignalRow(signal);
        } catch (err) {
            console.error('Error parsing signal:', err);
        }
    });

    strategyEventSource.addEventListener('error', (e) => {
        statusEl.textContent = 'Reconnecting';
        indicatorEl.style.backgroundColor = '#F6465D';
        indicatorEl.style.animation = 'pulse 1s infinite';
    });
}

// ===== RENDERING FUNCTIONS =====
function renderSignalRow(signal, isInitialLoad = false) {
    const tbody = safeGetElement('signals-table-body', 'Render');
    const placeholder = safeGetElement('signals-placeholder', 'Render');

    // Check if tbody exists
    if (!tbody) {
        return;
    }

    const signalId = `${signal.stock_symbol}-${signal.strategy}-${signal.timestamp}`;
    if (renderedSignalIds.has(signalId)) return;

    // Hide placeholder if visible
    if (placeholder) placeholder.style.display = 'none';

    // Determine colors and classes for decision
    let badgeClass = 'badge';
    let decisionIcon = '';
    if (signal.decision === 'BUY') {
        badgeClass = 'badge buy';
        decisionIcon = '📈';
    } else if (signal.decision === 'SELL') {
        badgeClass = 'badge sell';
        decisionIcon = '📉';
    } else if (signal.decision === 'WAIT') {
        badgeClass = 'badge unknown';
        decisionIcon = '⏸️';
    }

    // Format data
    const price = new Intl.NumberFormat('id-ID').format(signal.price);
    const change = signal.change.toFixed(2);
    const changeSign = signal.change >= 0 ? '+' : '';
    const changeClass = signal.change >= 0 ? 'diff-positive' : 'diff-negative';
    const confidence = Math.round(signal.confidence * 100);

    // Confidence level with icon and color
    let confidenceClass = 'confidence-low';
    let confidenceIcon = '⚪';
    let confidenceLabel = 'Low';
    if (confidence >= 80) {
        confidenceClass = 'confidence-extreme';
        confidenceIcon = '🔴';
        confidenceLabel = 'Extreme';
    } else if (confidence >= 70) {
        confidenceClass = 'confidence-high';
        confidenceIcon = '🟠';
        confidenceLabel = 'High';
    } else if (confidence >= 50) {
        confidenceClass = 'confidence-medium';
        confidenceIcon = '🟡';
        confidenceLabel = 'Medium';
    }

    // Time formatting - human readable
    const timeAgo = getTimeAgo(new Date(signal.timestamp));
    const fullTime = new Date(signal.timestamp).toLocaleString('id-ID');

    // Z-Score indicators
    const priceZScore = signal.price_z_score || 0;
    const volumeZScore = signal.volume_z_score || 0;

    // Enhanced reason with Z-Score info
    let enhancedReason = signal.reason || '-';
    const zScoreInfo = `Price Z: ${priceZScore.toFixed(2)} | Vol Z: ${volumeZScore.toFixed(2)}`;

    // Create row
    const row = document.createElement('tr');
    row.innerHTML = `
        <td class="col-time" title="${fullTime}">${timeAgo}</td>
        <td class="col-symbol"><strong>${signal.stock_symbol}</strong></td>
        <td title="${signal.strategy.replace(/_/g, ' ')}">${formatStrategyName(signal.strategy)}</td>
        <td><span class="${badgeClass}">${decisionIcon} ${signal.decision}</span></td>
        <td class="col-price">Rp ${price}</td>
        <td class="text-right"><span class="${changeClass}">${changeSign}${change}%</span></td>
        <td class="text-right">
            <span class="${confidenceClass}" title="${confidenceLabel} Confidence (${confidence}%)">${confidenceIcon} ${confidence}%</span>
        </td>
        <td class="reason-cell" title="${zScoreInfo}">
            ${enhancedReason}
            ${priceZScore > 0 || volumeZScore > 0 ? `<div style="font-size:0.7em; color:#888; margin-top:4px;">${zScoreInfo}</div>` : ''}
        </td>
    `;

    // Add animation
    row.style.opacity = '0';
    row.style.transform = 'translateY(-10px)';

    // Insert position depends on context:
    // - Initial load: append to end (signals already sorted DESC)
    // - Real-time SSE: prepend to beginning (newest on top)
    if (isInitialLoad) {
        tbody.appendChild(row);
    } else {
        if (tbody.firstChild) {
            tbody.insertBefore(row, tbody.firstChild);
        } else {
            tbody.appendChild(row);
        }
    }

    // Trigger animation
    setTimeout(() => {
        row.style.transition = `all ${STRATEGY_CONFIG.TRANSITION_DURATION}ms ease`;
        row.style.opacity = '1';
        row.style.transform = 'translateY(0)';
    }, STRATEGY_CONFIG.ANIMATION_DELAY);

    renderedSignalIds.add(signalId);

    // Limit number of rows
    if (tbody.children.length > STRATEGY_CONFIG.MAX_VISIBLE_SIGNALS) {
        tbody.removeChild(tbody.lastChild);
    }
}


// ===== HELPER FUNCTIONS =====
function formatStrategyName(strategy) {
    return strategy.replace(/_/g, ' ').toLowerCase().replace(/\b\w/g, l => l.toUpperCase());
}

function getTimeAgo(date) {
    const seconds = Math.floor((new Date() - date) / 1000);

    if (seconds < 10) return 'Just now';
    if (seconds < 60) return `${seconds}s ago`;

    const minutes = Math.floor(seconds / 60);
    if (minutes < 60) return `${minutes}m ago`;

    const hours = Math.floor(minutes / 60);
    if (hours < 24) return `${hours}h ago`;

    const days = Math.floor(hours / 24);
    if (days === 1) return 'Yesterday';
    if (days < 7) return `${days}d ago`;

    return date.toLocaleDateString('id-ID', { month: 'short', day: 'numeric' });
}
