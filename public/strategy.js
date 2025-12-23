document.addEventListener('DOMContentLoaded', () => {
    initStrategySystem();
});

let strategyEventSource = null;
let activeStrategyFilter = 'ALL';
let renderedSignalIds = new Set();
const MAX_VISIBLE_SIGNALS = 100;

function initStrategySystem() {
    // Verify critical elements exist before initializing
    const tbody = document.getElementById('signals-table-body');
    if (!tbody) {
        console.error('Critical element missing: signals-table-body not found in DOM');
        return;
    }
    
    setupStrategyTabs();
    fetchInitialSignals();
    connectStrategySSE();
}

async function fetchInitialSignals() {
    const tbody = document.getElementById('signals-table-body');
    const placeholder = document.getElementById('signals-placeholder');
    const loading = document.getElementById('signals-loading');
    
    // Check if elements exist
    if (!tbody) {
        console.error('signals-table-body element not found');
        return;
    }
    
    if (placeholder) placeholder.style.display = 'none';
    if (loading) loading.style.display = 'flex';

    try {
        let url = '/api/strategies/signals?lookback=60';
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
                renderSignalRow(signal);
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
            const tbody = document.getElementById('signals-table-body');
            if (tbody) {
                tbody.innerHTML = '';
            }
            renderedSignalIds.clear();
            
            // Reconnect with new filter
            connectStrategySSE();
            fetchInitialSignals();
        });
    });
}

function connectStrategySSE() {
    if (strategyEventSource) {
        strategyEventSource.close();
    }

    const statusEl = document.getElementById('strategy-connection-status');
    const indicatorEl = document.getElementById('strategy-live-indicator');
    
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
        
        const placeholder = document.getElementById('signals-placeholder');
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

function renderSignalRow(signal) {
    const tbody = document.getElementById('signals-table-body');
    const placeholder = document.getElementById('signals-placeholder');
    
    // Check if tbody exists
    if (!tbody) {
        console.error('signals-table-body element not found');
        return;
    }
    
    const signalId = `${signal.stock_symbol}-${signal.strategy}-${signal.timestamp}`;
    if (renderedSignalIds.has(signalId)) return;

    // Hide placeholder if visible
    if (placeholder) placeholder.style.display = 'none';

    // Determine colors and classes
    let badgeClass = 'badge';
    if (signal.decision === 'BUY') {
        badgeClass = 'badge buy';
    } else if (signal.decision === 'SELL') {
        badgeClass = 'badge sell';
    } else if (signal.decision === 'WAIT') {
        badgeClass = 'badge unknown';
    }

    // Format data
    const price = new Intl.NumberFormat('id-ID').format(signal.price);
    const change = signal.change.toFixed(2);
    const changeSign = signal.change >= 0 ? '+' : '';
    const changeClass = signal.change >= 0 ? 'diff-positive' : 'diff-negative';
    const confidence = Math.round(signal.confidence * 100);
    
    // Confidence level
    let confidenceClass = '';
    if (confidence >= 80) {
        confidenceClass = 'value-highlight';
    }
    
    // Time formatting
    const timestamp = new Date(signal.timestamp);
    const timeStr = timestamp.toLocaleTimeString('id-ID', { 
        hour: '2-digit', 
        minute: '2-digit',
        second: '2-digit'
    });

    // Create row
    const row = document.createElement('tr');
    row.innerHTML = `
        <td class="col-time">${timeStr}</td>
        <td class="col-symbol">${signal.stock_symbol}</td>
        <td>${formatStrategyName(signal.strategy)}</td>
        <td><span class="${badgeClass}">${signal.decision}</span></td>
        <td class="col-price">Rp ${price}</td>
        <td class="text-right"><span class="${changeClass}">${changeSign}${change}%</span></td>
        <td class="text-right ${confidenceClass}">${confidence}%</td>
        <td class="reason-cell">${signal.reason || '-'}</td>
    `;

    // Add animation
    row.style.opacity = '0';
    row.style.transform = 'translateY(-10px)';
    
    // Insert at the beginning (newest first)
    if (tbody.firstChild) {
        tbody.insertBefore(row, tbody.firstChild);
    } else {
        tbody.appendChild(row);
    }

    // Trigger animation
    setTimeout(() => {
        row.style.transition = 'all 0.3s ease';
        row.style.opacity = '1';
        row.style.transform = 'translateY(0)';
    }, 10);

    renderedSignalIds.add(signalId);

    // Limit number of rows
    if (tbody.children.length > MAX_VISIBLE_SIGNALS) {
        tbody.removeChild(tbody.lastChild);
    }
}

function formatStrategyName(strategy) {
    return strategy.replace(/_/g, ' ').toLowerCase().replace(/\b\w/g, l => l.toUpperCase());
}
