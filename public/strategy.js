document.addEventListener('DOMContentLoaded', () => {
    initStrategySystem();
});

let strategyEventSource = null;
let activeStrategyFilter = 'ALL';
let renderedSignalIds = new Set();
const MAX_VISIBLE_SIGNALS = 50;

function initStrategySystem() {
    setupStrategyTabs();
    fetchInitialSignals();
    connectStrategySSE();
}

async function fetchInitialSignals() {
    const container = document.getElementById('signals-container');
    if (!container.querySelector('.signal-card') && !container.querySelector('.placeholder')) {
         container.innerHTML = `
            <div class="placeholder">
                <span class="placeholder-icon">⏳</span>
                <p>Loading recent signals...</p>
            </div>
        `;
    }

    try {
        let url = '/api/strategies/signals?lookback=60';
        if (activeStrategyFilter !== 'ALL') {
            url += `&strategy=${activeStrategyFilter}`;
        }

        const res = await fetch(url);
        const data = await res.json();
        
        if (data.signals && data.signals.length > 0) {
            const placeholder = container.querySelector('.placeholder');
            if (placeholder) placeholder.remove();

            const signals = data.signals.sort((a, b) => new Date(a.timestamp) - new Date(b.timestamp));

            signals.forEach(signal => {
                renderSignalCard(signal);
            });
        } else {
             if (!container.querySelector('.signal-card')) {
                container.innerHTML = `
                    <div class="placeholder">
                        <span class="placeholder-icon">📡</span>
                        <p>No recent signals found.</p>
                    </div>
                `;
            }
        }
    } catch (err) {
        console.error("Failed to fetch initial signals:", err);
    }
}

function setupStrategyTabs() {
    const tabs = document.querySelectorAll('.strategy-tab');
    
    const icons = {
        'ALL': '📋',
        'VOLUME_BREAKOUT': '🚀',
        'MEAN_REVERSION': '↩️',
        'FAKEOUT_FILTER': '🛡️'
    };

    tabs.forEach(tab => {
        const strategy = tab.dataset.strategy;
        if (!tab.innerHTML.includes('icon')) {
            const icon = icons[strategy] || '📊';
            const text = tab.innerText;
            tab.innerHTML = `<span>${icon}</span> ${text}`;
        }

        tab.addEventListener('click', () => {
            tabs.forEach(t => t.classList.remove('active'));
            tab.classList.add('active');
            activeStrategyFilter = tab.dataset.strategy;
            connectStrategySSE();
            
            const container = document.getElementById('signals-container');
            container.style.opacity = '0.5';
            setTimeout(() => {
                container.innerHTML = `
                    <div class="placeholder">
                        <span class="placeholder-icon">📡</span>
                        <p>Filtering signals...</p>
                    </div>
                `;
                container.style.opacity = '1';
                renderedSignalIds.clear();
                fetchInitialSignals();
            }, 200);
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
        
        const container = document.getElementById('signals-container');
        if (container.querySelector('.placeholder')) {
            container.innerHTML = '';
        }
    });

    strategyEventSource.addEventListener('signal', (e) => {
        try {
            const signal = JSON.parse(e.data);
            renderSignalCard(signal);
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

function renderSignalCard(signal) {
    const container = document.getElementById('signals-container');
    
    const signalId = `${signal.stock_symbol}-${signal.strategy}-${signal.timestamp}`;
    if (renderedSignalIds.has(signalId)) return;

    // Determine colors
    let actionColor = '#707a8a';
    let cardClass = '';

    if (signal.decision === 'BUY') {
        actionColor = '#0ECB81';
        cardClass = 'buy-signal';
    } else if (signal.decision === 'SELL') {
        actionColor = '#F6465D';
        cardClass = 'sell-signal';
    } else if (signal.decision === 'WAIT') {
        actionColor = '#FFD700';
        cardClass = 'wait-signal';
    }

    // Format data
    const price = new Intl.NumberFormat('id-ID').format(signal.price);
    const change = signal.change.toFixed(2);
    const changeSign = signal.change >= 0 ? '+' : '';
    const changeColor = signal.change >= 0 ? '#0ECB81' : '#F6465D';
    const confidence = Math.round(signal.confidence * 100);
    
    // Time
    const timeAgo = getTimeAgo(new Date(signal.timestamp));

    const card = document.createElement('div');
    card.className = `signal-card ${cardClass}`;
    
    card.innerHTML = `
        <div class="card-header">
            <div class="card-symbol">${signal.stock_symbol}</div>
            <div class="card-action" style="background: ${actionColor}20; color: ${actionColor}; border-color: ${actionColor}40">
                ${signal.decision}
            </div>
        </div>
        
        <div class="card-body">
            <div class="card-info">
                <div class="info-item">
                    <span class="info-label">Price</span>
                    <span class="info-value">Rp ${price}</span>
                </div>
                <div class="info-item">
                    <span class="info-label">Change</span>
                    <span class="info-value" style="color: ${changeColor}">${changeSign}${change}%</span>
                </div>
                <div class="info-item">
                    <span class="info-label">Confidence</span>
                    <span class="info-value" style="color: ${actionColor}">${confidence}%</span>
                </div>
            </div>
        </div>

        <div class="card-footer">
            <span class="card-strategy">${formatStrategyName(signal.strategy)}</span>
            <span class="card-time">${timeAgo}</span>
        </div>
    `;

    if (container.firstChild) {
        container.insertBefore(card, container.firstChild);
    } else {
        container.appendChild(card);
    }

    renderedSignalIds.add(signalId);

    if (container.children.length > MAX_VISIBLE_SIGNALS) {
        if (container.lastChild) container.removeChild(container.lastChild);
    }
}

function formatStrategyName(strategy) {
    return strategy.replace(/_/g, ' ').toLowerCase().replace(/\b\w/g, l => l.toUpperCase());
}

function getTimeAgo(date) {
    const seconds = Math.floor((new Date() - date) / 1000);
    if (seconds < 60) return 'Just now';
    const minutes = Math.floor(seconds / 60);
    if (minutes < 60) return `${minutes}m ago`;
    const hours = Math.floor(minutes / 60);
    if (hours < 24) return `${hours}h ago`;
    return date.toLocaleTimeString([], {hour: '2-digit', minute:'2-digit'});
}
