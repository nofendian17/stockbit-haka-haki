document.addEventListener('DOMContentLoaded', () => {
    initStrategySystem();
});

let strategyEventSource = null;
let activeStrategyFilter = 'ALL';
let renderedSignalIds = new Set();
const MAX_VISIBLE_SIGNALS = 50;

function initStrategySystem() {
    setupStrategyTabs();
    fetchInitialSignals(); // Fetch initial data
    connectStrategySSE();
}

async function fetchInitialSignals() {
    const container = document.getElementById('signals-container');
    // Don't clear if it's the first load, but show loading if needed
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
            // Remove placeholder if exists
            const placeholder = container.querySelector('.placeholder');
            if (placeholder) placeholder.remove();

            // Reverse to show oldest first, so they are prepended in correct order (newest on top)
            // Actually renderSignalCardRefactored prepends, so we should process from oldest to newest
            // to end up with newest on top.
            // Assuming API returns newest first? Let's check api/server.go...
            // Standard SQL/GORM usually returns arbitrary unless ordered.
            // Let's assume we need to sort or just render.
            // If renderSignalCardRefactored prepends, we want to render the OLDEST first, 
            // so the newest ends up at the top.
            
            // Let's just process them.
            // If the API returns sorted by time desc (newest first), we should iterate in reverse.
            
            const signals = data.signals.sort((a, b) => new Date(a.timestamp) - new Date(b.timestamp));

            signals.forEach(signal => {
                renderSignalCardRefactored(signal);
            });
        } else {
             if (!contatiner.querySelector('.signal-card')) {
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
    
    // Add icons to tabs dynamically if not present
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
            // Update active state
            tabs.forEach(t => t.classList.remove('active'));
            tab.classList.add('active');

            // Update filter
            activeStrategyFilter = tab.dataset.strategy;
            
            // Reconnect SSE with new filter
            connectStrategySSE();
            
            // Clear current signals with animation
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
                fetchInitialSignals(); // Fetch for new filter
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
            renderSignalCardRefactored(signal);
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

function renderSignalCardRefactored(signal) {
    const container = document.getElementById('signals-container');
    
    const signalId = `${signal.stock_symbol}-${signal.strategy}-${signal.timestamp}`;
    if (renderedSignalIds.has(signalId)) return;

    // Determine colors and labels
    let actionColor = '#707a8a';
    let actionLabel = signal.decision;
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

    // Format Data
    const priceFormatted = new Intl.NumberFormat('id-ID').format(signal.price);
    const changeFormatted = Math.abs(signal.change).toFixed(2);
    const changeSign = signal.change >= 0 ? '+' : '-';
    const changeColor = signal.change >= 0 ? '#0ECB81' : '#F6465D';
    
    // Confidence Meter
    const confidencePercent = Math.round(signal.confidence * 100);

    // Simplified Reason (Extract key info)
    let simpleReason = signal.reason;
    // Highlight key terms
    simpleReason = simpleReason.replace(/(Z=[\d\.]+)/g, '<span class="signal-reason-highlight">$1</span>');
    simpleReason = simpleReason.replace(/([0-9\.]+%)/g, '<span class="signal-reason-highlight">$1</span>');

    // Time Ago
    const timeAgo = getTimeAgo(new Date(signal.timestamp));

    const card = document.createElement('div');
    card.className = `signal-card ${cardClass}`;
    
    card.innerHTML = `
        <div class="signal-top-bar">
            <div class="signal-action-badge">
                <span class="${signal.decision.toLowerCase()}-text">${actionLabel}</span>
            </div>
            <div class="signal-time">${timeAgo}</div>
        </div>
        
        <div class="signal-content-body">
            <div class="signal-info">
                <div class="signal-symbol-row">
                    <span class="signal-symbol-main">${signal.stock_symbol}</span>
                    <span class="signal-price-main">${priceFormatted}</span>
                    <span class="signal-change-pill" style="background: ${changeColor}20; color: ${changeColor}">
                        ${changeSign}${changeFormatted}%
                    </span>
                </div>
                
                <div class="signal-reason-text">
                    ${simpleReason}
                </div>
                
                <span class="strategy-tag">${formatStrategyName(signal.strategy)}</span>
            </div>

            <div class="signal-confidence-box">
                <div class="con-ring" style="--c-percent: ${confidencePercent}%; --c-color: ${actionColor}">
                    <span class="con-val">${confidencePercent}%</span>
                </div>
                <span class="con-label">Conf</span>
            </div>
        </div>
    `;

    if (container.firstChild) {
        container.insertBefore(card, container.firstChild);
    } else {
        container.appendChild(card);
    }

    renderedSignalIds.add(signalId);

    if (container.children.length > MAX_VISIBLE_SIGNALS) {
        container.removeChild(container.lastChild);
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
