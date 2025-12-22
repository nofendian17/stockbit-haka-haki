// Dashboard JavaScript - Trading Cockpit

// API Base URL
const API_BASE = window.location.origin;

// State Management
const state = {
  currentView: 'cockpit',
  pressureWindow: 5,
  whaleThreshold: 500000000, // 500 million IDR
  chartSymbol: '',
  chart: null,
  candleSeries: null,
  vwapSeries: null,
  whaleLines: [], // Track whale alert price lines
  streamController: null,
};

// Initialize Dashboard
document.addEventListener('DOMContentLoaded', () => {
  initializeNavigation();
  initializeCockpitView();
  initializeAnalysisView();
  initializeChartsView();
  
  // 🚀 Auto-load data for better UX
  console.log('🚀 Auto-loading initial data...');
  
  // Load cockpit data automatically
  loadLiveTrades();
  loadPressureGauge();
  loadWhaleAlerts();
  
  // Pre-fill chart symbol for quick demo
  const chartInput = document.getElementById('chart-symbol-input');
  if (chartInput) {
    chartInput.placeholder = 'Contoh: BBRI, TLKM, ASII';
  }
  
  // Start real-time updates
  startRealtimeUpdates();
  
  // Show welcome tooltip after 1 second
  setTimeout(() => {
    showWelcomeGuide();
  }, 1000);
});

// Navigation
function initializeNavigation() {
  document.querySelectorAll('.nav-btn').forEach(btn => {
    btn.addEventListener('click', (e) => {
      if (btn.dataset.view) {
        e.preventDefault();
        switchView(btn.dataset.view);
      }
    });
  });
}

function switchView(viewName) {
  // Update nav buttons
  document.querySelectorAll('.nav-btn').forEach(btn => {
    btn.classList.toggle('active', btn.dataset.view === viewName);
  });

  // Update views
  document.querySelectorAll('.dashboard-view').forEach(view => {
    view.classList.remove('active');
  });
  document.getElementById(`${viewName}-view`).classList.add('active');

  state.currentView = viewName;

  // Load view-specific data
  loadViewData(viewName);
}

function loadViewData(viewName) {
  if (viewName === 'cockpit') {
    loadLiveTrades();
    loadPressureGauge();
    loadWhaleAlerts();
  } else if (viewName === 'analysis') {
    loadVolumeSpikes();
    loadZScoreRanking();
    loadPowerCandles();
  }
  // Charts are loaded on-demand
}

// ===== SCALPING COCKPIT =====

function initializeCockpitView() {
  // Trade filters
  document.getElementById('trade-symbol-filter').addEventListener('input', debounce(loadLiveTrades, 300));
  document.getElementById('trade-action-filter').addEventListener('change', loadLiveTrades);
  
  // Pressure window selector
  document.getElementById('pressure-window').addEventListener('change', (e) => {
    state.pressureWindow = parseInt(e.target.value);
    loadPressureGauge();
  });
}

async function loadLiveTrades() {
  const symbol = document.getElementById('trade-symbol-filter').value.trim().toUpperCase();
  const action = document.getElementById('trade-action-filter').value;

  try {
    const params = new URLSearchParams({
      limit: '50',
    });
    if (symbol) params.append('symbol', symbol);
    if (action) params.append('action', action);

    const response = await fetch(`${API_BASE}/api/dashboard/live-trades?${params}`);
    const data = await response.json();

    displayLiveTrades(data.data || []);
  } catch (error) {
    console.error('Failed to load live trades:', error);
    showError('live-trades-list', 'Failed to load trades');
  }
}

function displayLiveTrades(trades) {
  const container = document.getElementById('live-trades-list');
  
  if (!trades || trades.length === 0) {
    container.innerHTML = '<div class="empty-state">No trades found</div>';
    return;
  }

  container.innerHTML = trades.map(trade => {
    const action = (trade.action || 'UNKNOWN').toLowerCase();
    const stockSymbol = trade.stock_symbol || trade.StockSymbol || 'N/A';
    const timestamp = trade.timestamp || trade.Timestamp || new Date();
    const price = trade.price || trade.Price || 0;
    const volumeLot = trade.volume_lot || trade.VolumeLot || 0;
    const totalAmount = trade.total_amount || trade.TotalAmount || 0;
    
    return `
      <div class="trade-item ${action}">
        <div>
          <div class="trade-time">${formatTime(timestamp)}</div>
          <div class="trade-symbol">${stockSymbol}</div>
        </div>
        <div class="trade-action ${action}">${trade.action || 'UNKNOWN'}</div>
        <div>
          <div class="trade-price">Rp ${formatNumber(price)}</div>
          <div class="trade-volume">${formatNumber(volumeLot)} lot</div>
        </div>
        <div class="trade-value">${formatCurrency(totalAmount)}</div>
      </div>
    `;
  }).join('');
}

async function loadPressureGauge() {
  try {
    const response = await fetch(`${API_BASE}/api/dashboard/pressure-gauge?window=${state.pressureWindow}`);
    const data = await response.json();

    if (!data) {
      console.warn('No pressure gauge data received');
      displayEmptyPressure();
      return;
    }

    if (Array.isArray(data)) {
      // Multiple stocks
      if (data.length === 0) {
        displayEmptyPressure();
        return;
      }
      displayPressureMetrics(aggregatePressure(data));
      drawPressureGauge(aggregatePressure(data));
    } else {
      // Single stock or overall
      displayPressureMetrics(data);
      drawPressureGauge(data);
    }
  } catch (error) {
    console.error('Failed to load pressure gauge:', error);
    displayEmptyPressure();
  }
}

function aggregatePressure(pressures) {
  const total = pressures.reduce((acc, p) => {
    acc.buyVolume += p.buy_volume;
    acc.sellVolume += p.sell_volume;
    acc.totalVolume += p.total_volume;
    return acc;
  }, { buyVolume: 0, sellVolume: 0, totalVolume: 0 });

  const buyPressurePct = total.totalVolume > 0 ? (total.buyVolume / total.totalVolume * 100) : 0;
  const sentiment = buyPressurePct > 60 ? 'BULLISH' : buyPressurePct < 40 ? 'BEARISH' : 'NEUTRAL';

  return {
    buy_volume: total.buyVolume,
    sell_volume: total.sellVolume,
    total_volume: total.totalVolume,
    buy_pressure_pct: buyPressurePct,
    market_sentiment: sentiment,
  };
}

function displayPressureMetrics(pressure) {
  const container = document.getElementById('pressure-metrics');
  
  // Check if pressure is array (multiple symbols) or single object
  if (Array.isArray(pressure)) {
    // Display top symbols with their pressure
    if (pressure.length === 0) {
      displayEmptyPressure();
      return;
    }
    
    container.innerHTML = pressure.map(p => {
      const sentiment = p.sentiment || p.market_sentiment || 'NEUTRAL';
      const sentimentClass = sentiment.toLowerCase();
      const buyPct = p.buy_pressure_pct || p.BuyPressurePct || 0;
      const symbol = p.stock_symbol || p.StockSymbol || 'N/A';
      
      return `
        <div class="pressure-symbol-item">
          <div class="pressure-symbol-header">
            <span class="pressure-symbol-name">${symbol}</span>
            <span class="pressure-sentiment ${sentimentClass}">${sentiment}</span>
          </div>
          <div class="pressure-bar-container">
            <div class="pressure-bar ${sentimentClass}" style="width: ${buyPct}%"></div>
            <div class="pressure-bar-label">${buyPct.toFixed(1)}%</div>
          </div>
          <div class="pressure-volumes">
            <span class="volume-buy">BUY: ${formatNumber(p.buy_volume || p.BuyVolume || 0)}</span>
            <span class="volume-sell">SELL: ${formatNumber(p.sell_volume || p.SellVolume || 0)}</span>
          </div>
        </div>
      `;
    }).join('');
    
    // Also update gauge with top symbol
    drawPressureGauge(pressure);
    
  } else if (pressure && (pressure.market_sentiment || pressure.sentiment)) {
    // Single symbol display (legacy support)
    const sentiment = pressure.market_sentiment || pressure.sentiment || 'NEUTRAL';
    const sentimentClass = sentiment.toLowerCase();
    const buyPct = pressure.buy_pressure_pct || pressure.BuyPressurePct || 0;
    
    container.innerHTML = `
      <div class="metric-item">
        <span class="metric-label">Buy Volume</span>
        <span class="metric-value">${formatNumber(pressure.buy_volume || 0)} lot</span>
      </div>
      <div class="metric-item">
        <span class="metric-label">Sell Volume</span>
        <span class="metric-value">${formatNumber(pressure.sell_volume || 0)} lot</span>
      </div>
      <div class="metric-item">
        <span class="metric-label">Buy Pressure</span>
        <span class="metric-value ${sentimentClass}">${buyPct.toFixed(1)}%</span>
      </div>
      <div class="metric-item">
        <span class="metric-label">Sentiment</span>
        <span class="metric-value ${sentimentClass}">${sentiment}</span>
      </div>
    `;
    
    drawPressureGauge(pressure);
  } else {
    displayEmptyPressure();
  }
}

function displayEmptyPressure() {
  const container = document.getElementById('pressure-metrics');
  container.innerHTML = `
    <div class="empty-state" style="grid-column: 1 / -1; text-align: center; padding: 2rem;">
      <p style="color: rgba(255,255,255,0.5);">⏳ Waiting for trading activity...</p>
    </div>
  `;
  
  // Clear gauge
  const svg = document.getElementById('gauge-svg');
  if (svg) {
    svg.innerHTML = '';
  }
}

function drawPressureGauge(pressure) {
  const svg = document.getElementById('gauge-svg');
  if (!svg) return;
  
  const width = svg.clientWidth;
  const height = 200;
  
  // Clear existing
  svg.innerHTML = '';

  // Get buy pressure percentage
  let buyPct = 50; // default neutral
  let symbolName = 'Market';
  
  if (Array.isArray(pressure) && pressure.length > 0) {
    // Show gauge for top symbol (most active)
    const top = pressure[0];
    buyPct = top.buy_pressure_pct || top.BuyPressurePct || 50;
    symbolName = top.stock_symbol || top.StockSymbol || 'Top';
  } else if (pressure) {
    buyPct = pressure.buy_pressure_pct || pressure.BuyPressurePct || 50;
    symbolName = pressure.stock_symbol || pressure.StockSymbol || 'Market';
  }

  // Draw arc gauge
  const centerX = width / 2;
  const centerY = height - 40;
  const radius = Math.min(width, height) / 2 - 40;

  // Background arc
  const bgArc = describeArc(centerX, centerY, radius, 180, 0);
  const path1 = document.createElementNS('http://www.w3.org/2000/svg', 'path');
  path1.setAttribute('d', bgArc);
  path1.setAttribute('fill', 'none');
  path1.setAttribute('stroke', 'rgba(255,255,255,0.1)');
  path1.setAttribute('stroke-width', '20');
  svg.appendChild(path1);

  // Pressure arc
  const angle = 180 - (buyPct / 100 * 180);
  const pressureArc = describeArc(centerX, centerY, radius, 180, angle);
  const path2 = document.createElementNS('http://www.w3.org/2000/svg', 'path');
  path2.setAttribute('d', pressureArc);
  path2.setAttribute('fill', 'none');
  
  // Color based on pressure
  let color = '#ffa502'; // neutral
  if (buyPct > 60) color = '#00ff88'; // bullish
  else if (buyPct < 40) color = '#ff4757'; // bearish
  
  path2.setAttribute('stroke', color);
  path2.setAttribute('stroke-width', '20');
  path2.setAttribute('stroke-linecap', 'round');
  svg.appendChild(path2);

  // Center text
  const text = document.createElementNS('http://www.w3.org/2000/svg', 'text');
  text.setAttribute('x', centerX);
  text.setAttribute('y', centerY - 10);
  text.setAttribute('text-anchor', 'middle');
  text.setAttribute('fill', '#fff');
  text.setAttribute('font-size', '36');
  text.setAttribute('font-weight', 'bold');
  text.textContent = `${buyPct.toFixed(0)}%`;
  svg.appendChild(text);

  // Symbol name
  const symbolText = document.createElementNS('http://www.w3.org/2000/svg', 'text');
  symbolText.setAttribute('x', centerX);
  symbolText.setAttribute('y', centerY + 20);
  symbolText.setAttribute('text-anchor', 'middle');
  symbolText.setAttribute('fill', 'rgba(255,255,255,0.7)');
  symbolText.setAttribute('font-size', '14');
  symbolText.textContent = symbolName;
  svg.appendChild(symbolText);

  // Labels
  const sellLabel = document.createElementNS('http://www.w3.org/2000/svg', 'text');
  sellLabel.setAttribute('x', 20);
  sellLabel.setAttribute('y', height - 10);
  sellLabel.setAttribute('fill', '#ff4757');
  sellLabel.setAttribute('font-size', '12');
  sellLabel.textContent = 'SELL';
  svg.appendChild(sellLabel);

  const buyLabel = document.createElementNS('http://www.w3.org/2000/svg', 'text');
  buyLabel.setAttribute('x', width - 40);
  buyLabel.setAttribute('y', height - 10);
  buyLabel.setAttribute('fill', '#00ff88');
  buyLabel.setAttribute('font-size', '12');
  buyLabel.textContent = 'BUY';
  svg.appendChild(buyLabel);
}


function describeArc(x, y, radius, startAngle, endAngle) {
  const start = polarToCartesian(x, y, radius, endAngle);
  const end = polarToCartesian(x, y, radius, startAngle);
  const largeArcFlag = endAngle - startAngle <= 180 ? '0' : '1';
  return [
    'M', start.x, start.y,
    'A', radius, radius, 0, largeArcFlag, 0, end.x, end.y
  ].join(' ');
}

function polarToCartesian(centerX, centerY, radius, angleInDegrees) {
  const angleInRadians = (angleInDegrees - 90) * Math.PI / 180.0;
  return {
    x: centerX + (radius * Math.cos(angleInRadians)),
    y: centerY + (radius * Math.sin(angleInRadians))
  };
}

async function loadWhaleAlerts() {
  try {
    const response = await fetch(`${API_BASE}/api/dashboard/whale-detector?threshold=${state.whaleThreshold}&limit=20`);
    const data = await response.json();

    displayWhaleAlerts(data.data || []);
    updateWhaleThresholdDisplay();
  } catch (error) {
    console.error('Failed to load whale alerts:', error);
  }
}

function displayWhaleAlerts(alerts) {
  const container = document.getElementById('whale-alerts-list');

  if (!alerts || alerts.length === 0) {
    container.innerHTML = '<div class="empty-state"><div class="empty-state-icon">🐋</div><p>No whale alerts detected</p></div>';
    return;
  }

  container.innerHTML = alerts.map(alert => {
    const stockSymbol = alert.stock_symbol || alert.StockSymbol || 'N/A';
    const timestamp = alert.timestamp || alert.Timestamp || alert.detected_at || alert.DetectedAt || new Date();
    const action = alert.action || alert.Action || 'UNKNOWN';
    const price = alert.price || alert.Price || alert.trigger_price || alert.TriggerPrice || 0;
    const totalAmount = alert.total_amount || alert.TotalAmount || alert.trigger_value || alert.TriggerValue || 0;
    const volumeLot = alert.volume_lot || alert.VolumeLot || alert.volume || alert.Volume || 0;
    const marketBoard = alert.market_board || alert.MarketBoard || 'N/A';
    
    return `
      <div class="whale-alert-item">
        <div class="whale-header">
          <div class="whale-symbol">${stockSymbol}</div>
          <div class="whale-time">${formatTime(timestamp)}</div>
        </div>
        <div class="whale-details">
          <div class="whale-detail"><strong>${action}</strong> @ Rp ${formatNumber(price)}</div>
          <div class="whale-detail"><strong>${formatCurrency(totalAmount)}</strong></div>
          <div class="whale-detail">${formatNumber(volumeLot)} lot</div>
          <div class="whale-detail">${marketBoard}</div>
        </div>
      </div>
    `;
  }).join('');
}

function updateWhaleThresholdDisplay() {
  const display = document.getElementById('whale-threshold-display');
  if (state.whaleThreshold >= 1000000000) {
    display.textContent = `${(state.whaleThreshold / 1000000000).toFixed(1)}B`;
  } else {
    display.textContent = `${(state.whaleThreshold / 1000000).toFixed(0)}M`;
  }
}

// ===== MOMENTUM ANALYSIS =====

function initializeAnalysisView() {
  document.getElementById('refresh-spikes-btn').addEventListener('click', loadVolumeSpikes);
  document.getElementById('refresh-candles-btn').addEventListener('click', loadPowerCandles);
  document.getElementById('zscore-min').addEventListener('change', loadZScoreRanking);
}

async function loadVolumeSpikes() {
  try {
    const response = await fetch(`${API_BASE}/api/dashboard/volume-spikes?min_spike=500`);
    const data = await response.json();

    displayVolumeSpikes(data.data || []);
  } catch (error) {
    console.error('Failed to load volume spikes:', error);
  }
}

function displayVolumeSpikes(spikes) {
  const container = document.getElementById('volume-spikes-list');

  if (!spikes || spikes.length === 0) {
    container.innerHTML = '<div class="empty-state">No volume spikes detected</div>';
    return;
  }

  container.innerHTML = spikes.map(spike => `
    <div class="spike-item">
      <div class="spike-header">
        <div class="spike-symbol">${spike.stock_symbol}</div>
        <div class="spike-pct">+${spike.spike_pct.toFixed(0)}%</div>
      </div>
      <div style="margin-top: 0.5rem;">
        <span class="alert-level ${spike.alert_level.toLowerCase()}">${spike.alert_level}</span>
        <div style="margin-top: 0.5rem; font-size: 0.85rem; color: rgba(255,255,255,0.7);">
          Current: ${formatNumber(spike.current_volume)} lot | Avg: ${formatNumber(spike.avg_volume_5m)} lot
        </div>
      </div>
    </div>
  `).join('');
}

async function loadZScoreRanking() {
  const minZ = parseFloat(document.getElementById('zscore-min').value);

  try {
    const response = await fetch(`${API_BASE}/api/dashboard/zscore-ranking?min_z=${minZ}&limit=20`);
    const data = await response.json();

    displayZScoreTable(data.data || []);
  } catch (error) {
    console.error('Failed to load Z-Score ranking:', error);
  }
}

function displayZScoreTable(rankings) {
  const container = document.getElementById('zscore-table-container');

  if (!rankings || rankings.length === 0) {
    container.innerHTML = '<div class="empty-state">No outliers found</div>';
    return;
  }

  container.innerHTML = `
    <table class="zscore-table">
      <thead>
        <tr>
          <th>#</th>
          <th>Symbol</th>
          <th>Z-Score</th>
          <th>Action</th>
          <th>Value</th>
        </tr>
      </thead>
      <tbody>
        ${rankings.map(item => `
          <tr>
            <td><span class="rank-badge">${item.rank}</span></td>
            <td><strong>${item.stock_symbol}</strong></td>
            <td><strong>${item.z_score.toFixed(2)}</strong></td>
            <td class="trade-action ${item.action.toLowerCase()}">${item.action}</td>
            <td>${formatCurrency(item.trigger_value)}</td>
          </tr>
        `).join('')}
      </tbody>
    </table>
  `;
}

async function loadPowerCandles() {
  try {
    const response = await fetch(`${API_BASE}/api/dashboard/power-candles?min_change=2&min_volume=500`);
    const data = await response.json();

    displayPowerCandles(data.data || []);
  } catch (error) {
    console.error('Failed to load power candles:', error);
  }
}

function displayPowerCandles(candles) {
  const container = document.getElementById('power-candles-list');

  if (!candles || candles.length === 0) {
    container.innerHTML = '<div class="empty-state">No power candles detected</div>';
    return;
  }

  container.innerHTML = candles.map(candle => {
    // Handle both flat and nested candle structure
    const stockSymbol = candle.stock_symbol || candle.StockSymbol || 'N/A';
    const open = candle.open || candle.Open || 0;
    const high = candle.high || candle.High || 0;
    const low = candle.low || candle.Low || 0;
    const close = candle.close || candle.Close || 0;
    const volumeLots = candle.volume_lots || candle.VolumeLots || 0;
    const priceChangePct = candle.price_change_pct || candle.PriceChangePct || 0;
    
    return `
      <div class="candle-item">
        <div class="candle-header">
          <div class="candle-symbol">${stockSymbol}</div>
          <div class="spike-pct ${priceChangePct > 0 ? 'trade-action buy' : 'trade-action sell'}">
            ${priceChangePct > 0 ? '+' : ''}${priceChangePct.toFixed(2)}%
          </div>
        </div>
        <div style="margin-top: 0.5rem; font-size: 0.85rem; color: rgba(255,255,255,0.7);">
          O: ${formatNumber(open)} | H: ${formatNumber(high)} | L: ${formatNumber(low)} | C: ${formatNumber(close)} | Vol: ${formatNumber(volumeLots)} lot
        </div>
      </div>
    `;
  }).join('');
}

// ===== CHARTS =====

function initializeChartsView() {
  document.getElementById('load-chart-btn').addEventListener('click', loadChart);
  document.getElementById('show-vwap').addEventListener('change', toggleVWAP);
  document.getElementById('show-whales').addEventListener('change', toggleWhaleMarkers);
}

async function loadChart() {
  const symbol = document.getElementById('chart-symbol-input').value.trim().toUpperCase();
  if (!symbol) {
    alert('Please enter a stock symbol');
    return;
  }

  state.chartSymbol = symbol;
  const hours = parseInt(document.getElementById('chart-timeframe').value);

  try {
    // Calculate time range
    const end = new Date();
    const start = new Date(end.getTime() - hours * 60 * 60 * 1000);

    const params = new URLSearchParams({
      start: start.toISOString(),
      end: end.toISOString(),
    });

    const response = await fetch(`${API_BASE}/api/dashboard/candles/${symbol}?${params}`);
    const data = await response.json();

    renderChart(data);

    // Load VWAP if enabled
    if (document.getElementById('show-vwap').checked) {
      loadVWAP(symbol, start);
    }
  } catch (error) {
    console.error('Failed to load chart:', error);
    alert('Failed to load chart data');
  }
}

function renderChart(chartData) {
  const container = document.getElementById('main-chart');
  container.innerHTML = ''; // Clear existing

  // Prepare candle data - Handle both lowercase and PascalCase
  const candles = chartData.candles || [];
  
  console.log('📊 [Chart] Processing', candles.length, 'candles');
  if (candles.length > 0) {
    console.log('📊 [Chart] Sample candle keys:', Object.keys(candles[0]));
  }

  // Destroy old chart
  if (state.chart) {
    state.chart.destroy();
    state.chart = null;
  }

  // Convert candle data to ApexCharts format
  const candleData = candles.map(c => {
    const bucket = c.bucket || c.Bucket;
    return {
      x: new Date(bucket),
      y: [
        c.open || c.Open || 0,
        c.high || c.High || 0,
        c.low || c.Low || 0,
        c.close || c.Close || 0,
      ]
    };
  });

  // Prepare whale annotations
  const whaleAnnotations = [];
  if (document.getElementById('show-whales').checked && chartData.whale_alerts) {
    chartData.whale_alerts.forEach(alert => {
      const timestamp = alert.detected_at || alert.DetectedAt || alert.timestamp || alert.Timestamp;
      const action = alert.action || alert.Action || 'UNKNOWN';
      const price = alert.trigger_price || alert.TriggerPrice || alert.price || alert.Price || 0;
      
      whaleAnnotations.push({
        x: new Date(timestamp).getTime(),
        y: price,
        marker: {
          size: 8,
          fillColor: action === 'BUY' ? '#00ff88' : '#ff4757',
          strokeColor: '#fff',
          strokeWidth: 2,
        },
        label: {
          text: '🐋',
          style: {
            fontSize: '20px',
            background: 'transparent',
          },
          offsetY: action === 'BUY' ? 20 : -20,
        }
      });
    });
  }

  console.log('🐋 [Whale] Adding', whaleAnnotations.length, 'whale annotations');

  // ApexCharts options
  const options = {
    series: [{
      name: 'candle',
      data: candleData
    }],
    chart: {
      type: 'candlestick',
      height: 500,
      background: 'rgba(0, 0, 0, 0.3)',
      foreColor: '#ffffff',
      toolbar: {
        show: true,
        tools: {
          zoom: true,
          zoomin: true,
          zoomout: true,
          pan: true,
          reset: true,
        }
      }
    },
    plotOptions: {
      candlestick: {
        colors: {
          upward: '#00ff88',
          downward: '#ff4757'
        },
        wick: {
          useFillColor: true
        }
      }
    },
    xaxis: {
      type: 'datetime',
      labels: {
        style: {
          colors: '#ffffff'
        },
        datetimeFormatter: {
          hour: 'HH:mm',
          minute: 'HH:mm'
        }
      }
    },
    yaxis: {
      tooltip: {
        enabled: true
      },
      labels: {
        style: {
          colors: '#ffffff'
        },
        formatter: (val) => 'Rp ' + formatNumber(val)
      }
    },
    grid: {
      borderColor: 'rgba(255, 255, 255, 0.1)',
    },
    tooltip: {
      theme: 'dark',
      custom: function({seriesIndex, dataPointIndex, w}) {
        const data = w.globals.initialSeries[seriesIndex].data[dataPointIndex];
        return `
          <div style="padding: 10px; background: rgba(0,0,0,0.9); border-radius: 4px;">
            <div style="margin-bottom: 5px;"><strong>Time:</strong> ${new Date(data.x).toLocaleTimeString('id-ID')}</div>
            <div><strong>Open:</strong> Rp ${formatNumber(data.y[0])}</div>
            <div><strong>High:</strong> Rp ${formatNumber(data.y[1])}</div>
            <div><strong>Low:</strong> Rp ${formatNumber(data.y[2])}</div>
            <div><strong>Close:</strong> Rp ${formatNumber(data.y[3])}</div>
          </div>
        `;
      }
    },
    annotations: {
      points: whaleAnnotations
    }
  };

  // Create chart
  state.chart = new ApexCharts(container, options);
  state.chart.render();

  console.log('✅ [Chart] ApexCharts candlestick rendered with', whaleAnnotations.length, 'whale markers');

  // Update legend
  updateChartLegend(chartData);
}

function addWhaleMarkers(alerts, candles) {
  // Whale markers are added in renderChart via annotations
  console.log('🐋 [Whale] Markers handled via ApexCharts annotations');
}

async function loadVWAP(symbol, startTime) {
  console.log('🔍 [VWAP] Loading VWAP for:', symbol, 'from:', startTime.toISOString());
  
  try {
    const url = `${API_BASE}/api/dashboard/vwap/${symbol}?start=${startTime.toISOString()}`;
    console.log('🔍 [VWAP] Fetching from:', url);
    
    const response = await fetch(url);
    const data = await response.json();
    
    console.log('🔍 [VWAP] Response data:', data);
    console.log('🔍 [VWAP] History length:', data.history ? data.history.length : 0);
    
    if (data.history && data.history.length > 0) {
      console.log('🔍 [VWAP] Sample data points:', data.history.slice(0, 3));
      addVWAPLine(data.history);
    } else {
      console.warn('⚠️ [VWAP] No history data received');
    }
  } catch (error) {
    console.error('❌ [VWAP] Failed to load:', error);
  }
}

function addVWAPLine(vwapHistory) {
  if (!state.chart || !vwapHistory || vwapHistory.length === 0) {
    console.warn('⚠️ [VWAP] No chart or empty VWAP history');
    return;
  }

  console.log('📊 [VWAP] Adding VWAP line with', vwapHistory.length, 'points');

  // Convert VWAP data to ApexCharts format
  const vwapData = vwapHistory.map(v => ({
    x: new Date(v.timestamp).getTime(),
    y: v.vwap || v.Vwap || 0,
  })).filter(v => v.y > 0);

  // Add VWAP as a line series
  state.chart.appendSeries({
    name: 'VWAP',
    type: 'line',
    data: vwapData,
    color: '#ffa502',
  });

  console.log('✅ [VWAP] Added', vwapData.length, 'VWAP points to chart');
}

function toggleVWAP() {
  if (!state.chartSymbol || !state.chart) return;

  if (document.getElementById('show-vwap').checked) {
    const hours = parseInt(document.getElementById('chart-timeframe').value);
    const start = new Date(Date.now() - hours * 60 * 60 * 1000);
    loadVWAP(state.chartSymbol, start);
  } else {
    // Remove VWAP series - reload chart without VWAP
    console.log('🗑️ [VWAP] Removing VWAP - reloading chart');
    // Simplest way: just reload the chart
    document.getElementById('load-chart-btn').click();
  }
}

function toggleWhaleMarkers() {
  if (state.chartSymbol) {
    loadChart(); // Reload chart with/without markers
  }
}

function updateChartLegend(chartData) {
  const legend = document.getElementById('chart-legend');
  const whaleCount = chartData.whale_alerts ? chartData.whale_alerts.length : 0;

  legend.innerHTML = `
    <strong>${state.chartSymbol}</strong> | 
    Candles: ${chartData.candles ? chartData.candles.length : 0} | 
    Whale Alerts: ${whaleCount}
  `;
}

// ===== REAL-TIME UPDATES =====

function startRealtimeUpdates() {
  console.log('🔴 [SSE] Starting Server-Sent Events for all dashboard data...');

  // Connect to comprehensive dashboard SSE endpoint
  connectDashboardSSE();

  // Also connect to whale alerts for push notifications
  connectWhaleStream();

  console.log('✅ [SSE] Full SSE system active - no polling!');
}

function connectDashboardSSE() {
  const eventSource = new EventSource(`${API_BASE}/api/dashboard/sse`);

  // COCKPIT VIEW EVENTS (5 second updates from backend)
  eventSource.addEventListener('live_trades', (event) => {
    if (state.currentView === 'cockpit') {
      try {
        const data = JSON.parse(event.data);
        displayLiveTrades(data.data || []);
        showUpdateIndicator();
      } catch (error) {
        console.error('[SSE] Error parsing live_trades:', error);
      }
    }
  });

  eventSource.addEventListener('pressure_gauge', (event) => {
    if (state.currentView === 'cockpit') {
      try {
        const pressure = JSON.parse(event.data);
        displayPressureMetrics(pressure);
        drawPressureGauge(pressure);
        showUpdateIndicator();
      } catch (error) {
        console.error('[SSE] Error parsing pressure_gauge:', error);
      }
    }
  });

  eventSource.addEventListener('whale_alerts', (event) => {
    if (state.currentView === 'cockpit') {
      try {
        const data = JSON.parse(event.data);
        displayWhaleAlerts(data.data || []);
        showUpdateIndicator();
      } catch (error) {
        console.error('[SSE] Error parsing whale_alerts:', error);
      }
    }
  });

  // ANALYSIS VIEW EVENTS (10 second updates from backend)
  eventSource.addEventListener('volume_spikes', (event) => {
    if (state.currentView === 'analysis') {
      try {
        const data = JSON.parse(event.data);
        displayVolumeSpikes(data.data || []);
        showUpdateIndicator();
      } catch (error) {
        console.error('[SSE] Error parsing volume_spikes:', error);
      }
    }
  });

  eventSource.addEventListener('zscore_ranking', (event) => {
    if (state.currentView === 'analysis') {
      try {
        const data = JSON.parse(event.data);
        displayZScoreTable(data.data || []);
        showUpdateIndicator();
      } catch (error) {
        console.error('[SSE] Error parsing zscore_ranking:', error);
      }
    }
  });

  eventSource.addEventListener('power_candles', (event) => {
    if (state.currentView === 'analysis') {
      try {
        const data = JSON.parse(event.data);
        displayPowerCandles(data.data || []);
        showUpdateIndicator();
      } catch (error) {
        console.error('[SSE] Error parsing power_candles:', error);
      }
    }
  });

  // Connection events
  eventSource.onopen = () => {
    console.log('✅ [SSE] Dashboard stream connected');
  };

  eventSource.onerror = (error) => {
    console.warn('⚠️ [SSE] Dashboard connection lost, auto-reconnecting...', error);
    // EventSource auto-reconnects
  };
}

function showUpdateIndicator() {
  let indicator = document.getElementById('update-indicator');
  if (!indicator) {
    indicator = document.createElement('div');
    indicator.id = 'update-indicator';
    indicator.style.cssText = `
      position: fixed;
      top: 80px;
      right: 20px;
      background: linear-gradient(135deg, #00ff88, #00d4aa);
      color: #000;
      padding: 8px 16px;
      border-radius: 20px;
      font-size: 0.85rem;
      font-weight: 600;
      z-index: 9999;
      box-shadow: 0 4px 12px rgba(0, 255, 136, 0.3);
    `;
    document.body.appendChild(indicator);
  }

  indicator.textContent = `🔄 ${new Date().toLocaleTimeString('id-ID')}`;
  indicator.style.display = 'block';

  setTimeout(() => indicator.style.display = 'none', 2000);
}

function connectWhaleStream() {
  const eventSource = new EventSource(`${API_BASE}/api/events`);

  eventSource.addEventListener('message', (event) => {
    try {
      const data = JSON.parse(event.data);
      if (data.event === 'whale_alert') {
        console.log('🐋 [SSE] New whale alert push notification!', data.payload);
        showWhaleNotification(data.payload);
      }
    } catch (error) {
      console.error('SSE parse error:', error);
    }
  });

  eventSource.onerror = () => {
    console.warn('Whale SSE connection lost, retrying...');
    setTimeout(() => {
      eventSource.close();
      connectWhaleStream();
    }, 5000);
  };
}

function showWhaleNotification(whale) {
  const toast = document.createElement('div');
  toast.style.cssText = `
    position: fixed;
    bottom: 20px;
    right: 20px;
    background: linear-gradient(135deg, rgba(102, 126, 234, 0.95), rgba(118, 75, 162, 0.95));
    color: white;
    padding: 1rem 1.5rem;
    border-radius: 12px;
    box-shadow: 0 8px 32px rgba(0, 0, 0, 0.3);
    z-index: 10000;
    max-width: 300px;
  `;

  const stockSymbol = whale.stock_symbol || whale.StockSymbol || 'N/A';
  const action = whale.action || whale.Action || 'UNKNOWN';
  const amount = whale.total_amount || whale.TotalAmount || whale.trigger_value || whale.TriggerValue || 0;

  toast.innerHTML = `
    <div style="display: flex; align-items: center; gap: 10px; margin-bottom: 8px;">
      <span style="font-size: 2rem;">🐋</span>
      <div>
        <div style="font-weight: 700; font-size: 1.1rem;">${stockSymbol}</div>
        <div style="font-size: 0.85rem; opacity: 0.9;">Whale Alert!</div>
      </div>
    </div>
    <div style="font-size: 0.9rem;">
      <strong style="color: ${action === 'BUY' ? '#00ff88' : '#ff4757'};">${action}</strong>
      <span style="margin-left: 8px;">${formatCurrency(amount)}</span>
    </div>
  `;

  document.body.appendChild(toast);
  setTimeout(() => toast.remove(), 5000);
}

// ===== UTILITY FUNCTIONS =====

function formatTime(timestamp) {
  const date = new Date(timestamp);
  return date.toLocaleTimeString('id-ID', { hour: '2-digit', minute: '2-digit', second: '2-digit' });
}

function formatNumber(num) {
  if (num === null || num === undefined) return '0';
  return new Intl.NumberFormat('id-ID').format(Math.floor(num));
}

function formatCurrency(amount) {
  if (amount === null || amount === undefined) return 'Rp 0';
  if (amount >= 1000000000) {
    return `Rp ${(amount / 1000000000).toFixed(2)}B`;
  } else if (amount >= 1000000) {
    return `Rp ${(amount / 1000000).toFixed(1)}M`;
  } else if (amount >= 1000) {
    return `Rp ${(amount / 1000).toFixed(0)}K`;
  }
  return `Rp ${formatNumber(amount)}`;
}

function debounce(func, wait) {
  let timeout;
  return function executedFunction(...args) {
    const later = () => {
      clearTimeout(timeout);
      func(...args);
    };
    clearTimeout(timeout);
    timeout = setTimeout(later, wait);
  };
}

function showError(containerId, message) {
  const container = document.getElementById(containerId);
  container.innerHTML = `<div class="empty-state" style="color: #ff4757;">${message}</div>`;
}

// Chart.js handles responsive sizing automatically
// No custom resize handler needed

// ===== UX IMPROVEMENTS =====

function showWelcomeGuide() {
  // Only show once per session
  if (sessionStorage.getItem('dashboard_visited')) return;
  sessionStorage.setItem('dashboard_visited', 'true');
  
  const guide = document.createElement('div');
  guide.style.cssText = `
    position: fixed;
    top: 50%;
    left: 50%;
    transform: translate(-50%, -50%);
    background: linear-gradient(135deg, rgba(102, 126, 234, 0.95), rgba(118, 75, 162, 0.95));
    padding: 2rem;
    border-radius: 12px;
    box-shadow: 0 10px 40px rgba(0, 0, 0, 0.5);
    z-index: 10000;
    max-width: 500px;
    color: white;
    animation: fadeIn 0.3s ease-in;
  `;
  
  guide.innerHTML = `
    <h3 style="margin: 0 0 1rem 0; font-size: 1.5rem;">👋 Selamat Datang di Trading Dashboard!</h3>
    <p style="margin: 0 0 1.5rem 0; line-height: 1.6;">
      <strong>Quick Start Guide:</strong><br><br>
      📊 <strong>Cockpit View:</strong> Monitor live trades & pressure gauge<br>
      📈 <strong>Analysis View:</strong> Lihat volume spikes & power candles<br>
      📉 <strong>Charts View:</strong> Analisa price chart + VWAP<br><br>
      💡 <strong>Tip:</strong> Klik "Load Sample Chart" untuk demo cepat!
    </p>
    <div style="display: flex; gap: 1rem;">
      <button onclick="loadQuickDemo()" style="
        flex: 1;
        padding: 0.8rem;
        background: white;
        color: #667eea;
        border: none;
        border-radius: 6px;
        font-weight: 600;
        cursor: pointer;
      ">📊 Load Sample Chart</button>
      <button onclick="this.parentElement.parentElement.remove()" style="
        flex: 1;
        padding: 0.8rem;
        background: rgba(255,255,255,0.2);
        color: white;
        border: 1px solid white;
        border-radius: 6px;
        font-weight: 600;
        cursor: pointer;
      ">Tutup</button>
    </div>
  `;
  
  document.body.appendChild(guide);
}

function loadQuickDemo() {
  // Remove welcome guide
  const guide = document.querySelector('[style*="z-index: 10000"]');
  if (guide) guide.remove();
  
  // Switch to Charts view
  switchView('charts');
  document.querySelectorAll('.nav-btn').forEach(btn => {
    btn.classList.toggle('active', btn.dataset.view === 'charts');
  });
  
  // Load BBRI chart
  setTimeout(() => {
    const chartInput = document.getElementById('chart-symbol-input');
    chartInput.value = 'BBRI';
    document.getElementById('load-chart-btn').click();
    
    // Enable VWAP after chart loads
    setTimeout(() => {
      const vwapCheckbox = document.getElementById('show-vwap');
      if (vwapCheckbox && !vwapCheckbox.checked) {
        vwapCheckbox.click();
      }
    }, 2000);
  }, 500);
}

function loadQuickSymbol(symbol) {
  const chartInput = document.getElementById('chart-symbol-input');
  chartInput.value = symbol;
  document.getElementById('load-chart-btn').click();
}
