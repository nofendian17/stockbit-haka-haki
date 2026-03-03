/**
 * AI Analysis Manager
 * Manages AI-powered symbol analysis and custom prompt interactions
 */

import { CONFIG } from './config.js';
import { safeGetElement, showToast, formatNumber } from './utils.js';
import { createAISymbolAnalysisStream, createAICustomPromptStream, fetchCandles } from './api.js';

let currentAnalysisStream = null;
let currentPromptController = null;
let isAnalyzing = false;
let currentSymbol = '';
let candleChart = null;

export function initAIAnalysis() {
    setupAIAnalysisModal();
    console.log('🤖 AI Analysis system initialized');
}

function setupAIAnalysisModal() {
    let modal = document.getElementById('ai-analysis-modal');
    if (!modal) {
        modal = createAIAnalysisModal();
        document.body.appendChild(modal);
    }

    const closeBtn = modal.querySelector('.ai-modal-close');
    const overlay = modal.querySelector('.ai-modal-overlay');
    
    if (closeBtn) closeBtn.addEventListener('click', closeAIAnalysisModal);
    if (overlay) overlay.addEventListener('click', closeAIAnalysisModal);

    setupTabNavigation();
    setupSymbolInput();
    setupQuickPickGrid();
}

function createAIAnalysisModal() {
    const modal = document.createElement('div');
    modal.id = 'ai-analysis-modal';
    modal.className = 'fixed inset-0 z-50 hidden';
    modal.innerHTML = `
        <div class="ai-modal-overlay absolute inset-0 bg-black/70 backdrop-blur-sm"></div>
        <div class="absolute inset-4 md:inset-8 bg-bgPrimary rounded-2xl border border-borderColor shadow-2xl flex flex-col overflow-hidden">
            <!-- Header -->
            <div class="flex items-center justify-between px-6 py-4 border-b border-borderColor bg-gradient-to-r from-bgSecondary to-bgPrimary">
                <div class="flex items-center gap-4">
                    <div class="w-12 h-12 rounded-xl bg-gradient-to-br from-purple-500 via-blue-500 to-cyan-500 flex items-center justify-center shadow-lg shadow-purple-500/20">
                        <span class="text-2xl">🤖</span>
                    </div>
                    <div>
                        <h2 class="text-xl font-bold text-textPrimary">AI Market Intelligence</h2>
                        <p class="text-xs text-textMuted flex items-center gap-2">
                            <span class="w-1.5 h-1.5 bg-accentSuccess rounded-full animate-pulse"></span>
                            Powered by Quantum Trader AI
                        </p>
                    </div>
                </div>
                <button class="ai-modal-close w-10 h-10 rounded-xl hover:bg-bgHover flex items-center justify-center text-textMuted hover:text-textPrimary transition-all">
                    <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"></path>
                    </svg>
                </button>
            </div>

            <!-- Tab Navigation -->
            <div class="flex border-b border-borderColor bg-bgSecondary/50 px-6">
                <button class="ai-tab-btn active px-6 py-3 text-sm font-semibold text-accentInfo border-b-2 border-accentInfo transition-all" data-tab="analysis">
                    <span class="flex items-center gap-2">
                        <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 19v-6a2 2 0 00-2-2H5a2 2 0 00-2 2v6a2 2 0 002 2h2a2 2 0 002-2zm0 0V9a2 2 0 012-2h2a2 2 0 012 2v10m-6 0a2 2 0 002 2h2a2 2 0 002-2m0 0V5a2 2 0 012-2h2a2 2 0 012 2v14a2 2 0 01-2 2h-2a2 2 0 01-2-2z"></path>
                        </svg>
                        Symbol Analysis
                    </span>
                </button>
                <button class="ai-tab-btn px-6 py-3 text-sm font-semibold text-textMuted hover:text-textPrimary transition-all" data-tab="chat">
                    <span class="flex items-center gap-2">
                        <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 12h.01M12 12h.01M16 12h.01M21 12c0 4.418-4.03 8-9 8a9.863 9.863 0 01-4.255-.949L3 20l1.395-3.72C3.512 15.042 3 13.574 3 12c0-4.418 4.03-8 9-8s9 3.582 9 8z"></path>
                        </svg>
                        AI Assistant
                    </span>
                </button>
            </div>

            <!-- Content Area -->
            <div class="flex-1 overflow-hidden flex flex-col lg:flex-row">
                <!-- Main Content -->
                <div class="flex-1 flex flex-col overflow-hidden">
                    <!-- Symbol Input (Analysis Tab) -->
                    <div id="analysis-input-section" class="p-4 border-b border-borderColor bg-bgSecondary/30">
                        <div class="flex gap-3">
                            <div class="flex-1 relative">
                                <input 
                                    type="text" 
                                    id="ai-symbol-input" 
                                    placeholder="Enter stock symbol (e.g., BBCA, TLKM, BBRI)..."
                                    class="w-full px-4 py-3 pl-10 bg-bgPrimary border border-borderColor rounded-xl text-textPrimary placeholder-textMuted focus:outline-none focus:border-accentInfo focus:ring-2 focus:ring-accentInfo/20 transition-all"
                                    maxlength="10"
                                >
                                <svg class="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-textMuted" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z"></path>
                                </svg>
                            </div>
                            <button 
                                id="ai-analyze-btn"
                                class="px-6 py-3 bg-gradient-to-r from-purple-600 to-blue-600 hover:from-purple-500 hover:to-blue-500 text-white font-semibold rounded-xl transition-all flex items-center gap-2 disabled:opacity-50 disabled:cursor-not-allowed shadow-lg shadow-purple-500/25"
                            >
                                <span>Analyze</span>
                                <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 10V3L4 14h7v7l9-11h-7z"></path>
                                </svg>
                            </button>
                        </div>
                    </div>

                    <!-- Analysis Content -->
                    <div id="ai-analysis-content" class="flex-1 overflow-y-auto p-6">
                        <div class="flex flex-col items-center justify-center h-full text-textMuted">
                            <div class="w-24 h-24 mb-6 rounded-2xl bg-gradient-to-br from-purple-500/20 to-blue-500/20 flex items-center justify-center">
                                <span class="text-5xl">📊</span>
                            </div>
                            <h3 class="text-xl font-semibold text-textPrimary mb-2">AI Market Analysis</h3>
                            <p class="text-center max-w-md mb-6">Enter a stock symbol to get comprehensive AI-powered analysis including whale flows, technical indicators, and trading signals.</p>
                            
                            <!-- Quick Pick Grid -->
                            <div class="grid grid-cols-4 gap-2 mt-4">
                                <button class="quick-symbol-btn px-4 py-2 bg-bgSecondary hover:bg-bgHover rounded-lg text-sm font-medium text-textPrimary transition-all hover:ring-2 hover:ring-accentInfo/50" data-symbol="BBCA">BBCA</button>
                                <button class="quick-symbol-btn px-4 py-2 bg-bgSecondary hover:bg-bgHover rounded-lg text-sm font-medium text-textPrimary transition-all hover:ring-2 hover:ring-accentInfo/50" data-symbol="TLKM">TLKM</button>
                                <button class="quick-symbol-btn px-4 py-2 bg-bgSecondary hover:bg-bgHover rounded-lg text-sm font-medium text-textPrimary transition-all hover:ring-2 hover:ring-accentInfo/50" data-symbol="BBRI">BBRI</button>
                                <button class="quick-symbol-btn px-4 py-2 bg-bgSecondary hover:bg-bgHover rounded-lg text-sm font-medium text-textPrimary transition-all hover:ring-2 hover:ring-accentInfo/50" data-symbol="BMRI">BMRI</button>
                                <button class="quick-symbol-btn px-4 py-2 bg-bgSecondary hover:bg-bgHover rounded-lg text-sm font-medium text-textPrimary transition-all hover:ring-2 hover:ring-accentInfo/50" data-symbol="ASII">ASII</button>
                                <button class="quick-symbol-btn px-4 py-2 bg-bgSecondary hover:bg-bgHover rounded-lg text-sm font-medium text-textPrimary transition-all hover:ring-2 hover:ring-accentInfo/50" data-symbol="UNVR">UNVR</button>
                                <button class="quick-symbol-btn px-4 py-2 bg-bgSecondary hover:bg-bgHover rounded-lg text-sm font-medium text-textPrimary transition-all hover:ring-2 hover:ring-accentInfo/50" data-symbol="HRUM">HRUM</button>
                                <button class="quick-symbol-btn px-4 py-2 bg-bgSecondary hover:bg-bgHover rounded-lg text-sm font-medium text-textPrimary transition-all hover:ring-2 hover:ring-accentInfo/50" data-symbol="ANTM">ANTM</button>
                            </div>
                        </div>
                    </div>

                    <!-- Chat Content (Hidden by default) -->
                    <div id="ai-chat-content" class="hidden flex-1 flex flex-col overflow-hidden">
                        <div id="ai-chat-messages" class="flex-1 overflow-y-auto p-4 space-y-4">
                            <div class="flex gap-3">
                                <div class="w-10 h-10 rounded-xl bg-gradient-to-br from-purple-500 to-blue-500 flex items-center justify-center flex-shrink-0">
                                    <span class="text-lg">🤖</span>
                                </div>
                                <div class="flex-1">
                                    <div class="bg-bgSecondary rounded-xl p-4 text-textSecondary text-sm">
                                        <p class="font-semibold text-textPrimary mb-2">AI Assistant Ready</p>
                                        <p>I can help you analyze market data, whale flows, and trading patterns. Try asking:</p>
                                        <ul class="mt-3 space-y-2 text-xs text-textMuted">
                                            <li class="flex items-start gap-2">
                                                <span class="text-accentInfo">▸</span>
                                                "Which stocks show strong accumulation today?"
                                            </li>
                                            <li class="flex items-start gap-2">
                                                <span class="text-accentInfo">▸</span>
                                                "What's the current trend for BBCA?"
                                            </li>
                                            <li class="flex items-start gap-2">
                                                <span class="text-accentInfo">▸</span>
                                                "Analyze buy/sell pressure in the last 4 hours"
                                            </li>
                                        </ul>
                                    </div>
                                </div>
                            </div>
                        </div>
                        <div class="p-4 border-t border-borderColor bg-bgSecondary/30">
                            <div class="flex gap-3">
                                <textarea 
                                    id="ai-chat-input" 
                                    placeholder="Ask me anything about the market..."
                                    class="flex-1 px-4 py-3 bg-bgPrimary border border-borderColor rounded-xl text-textPrimary placeholder-textMuted focus:outline-none focus:border-accentInfo focus:ring-2 focus:ring-accentInfo/20 transition-all resize-none"
                                    rows="2"
                                    maxlength="500"
                                ></textarea>
                                <button 
                                    id="ai-chat-send"
                                    class="px-5 py-3 bg-gradient-to-r from-green-600 to-cyan-600 hover:from-green-500 hover:to-cyan-500 text-white font-semibold rounded-xl transition-all flex items-center justify-center shadow-lg shadow-green-500/25"
                                >
                                    <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 19l9 2-9-18-9 18 9-2zm0 0v-8"></path>
                                    </svg>
                                </button>
                            </div>
                        </div>
                    </div>
                </div>

                <!-- Sidebar -->
                <div class="w-full lg:w-80 border-t lg:border-t-0 lg:border-l border-borderColor bg-bgSecondary/20 p-4 overflow-y-auto">
                    <h3 class="text-sm font-semibold text-textSecondary uppercase tracking-wider mb-4 flex items-center gap-2">
                        <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 7h8m0 0v8m0-8l-8 8-4-4-6 6"></path>
                        </svg>
                        Market Overview
                    </h3>
                    
                    <!-- Current Symbol Stats -->
                    <div id="symbol-stats" class="space-y-3">
                        <div class="bg-bgSecondary/50 rounded-xl p-4 border border-borderColor">
                            <div class="text-center py-8 text-textMuted">
                                <span class="text-4xl block mb-2">📈</span>
                                <p class="text-sm">Select a symbol to view analysis</p>
                            </div>
                        </div>
                    </div>

                    <!-- Analysis Features -->
                    <h3 class="text-sm font-semibold text-textSecondary uppercase tracking-wider mb-3 mt-6 flex items-center gap-2">
                        <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5H7a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2V7a2 2 0 00-2-2h-2M9 5a2 2 0 002 2h2a2 2 0 002-2M9 5a2 2 0 012-2h2a2 2 0 012 2"></path>
                        </svg>
                        What's Analyzed
                    </h3>
                    <div class="space-y-2 text-xs">
                        <div class="flex items-center gap-3 p-2 rounded-lg bg-bgSecondary/50">
                            <div class="w-8 h-8 rounded-lg bg-accentSuccess/20 flex items-center justify-center text-accentSuccess">🐋</div>
                            <span class="text-textSecondary">Whale Flow Analysis</span>
                        </div>
                        <div class="flex items-center gap-3 p-2 rounded-lg bg-bgSecondary/50">
                            <div class="w-8 h-8 rounded-lg bg-accentInfo/20 flex items-center justify-center text-accentInfo">📊</div>
                            <span class="text-textSecondary">Statistical Patterns</span>
                        </div>
                        <div class="flex items-center gap-3 p-2 rounded-lg bg-bgSecondary/50">
                            <div class="w-8 h-8 rounded-lg bg-accentWarning/20 flex items-center justify-center text-accentWarning">📈</div>
                            <span class="text-textSecondary">Order Flow Imbalance</span>
                        </div>
                        <div class="flex items-center gap-3 p-2 rounded-lg bg-bgSecondary/50">
                            <div class="w-8 h-8 rounded-lg bg-purple-500/20 flex items-center justify-center text-purple-400">🎯</div>
                            <span class="text-textSecondary">Historical Impact</span>
                        </div>
                    </div>
                </div>
            </div>
        </div>
    `;
    
    window.analyzeSymbol = analyzeSymbol;
    return modal;
}

function setupTabNavigation() {
    const tabs = document.querySelectorAll('.ai-tab-btn');
    const analysisSection = document.getElementById('analysis-input-section');
    const analysisContent = document.getElementById('ai-analysis-content');
    const chatContent = document.getElementById('ai-chat-content');
    
    tabs.forEach(tab => {
        tab.addEventListener('click', () => {
            tabs.forEach(t => {
                t.classList.remove('active', 'text-accentInfo', 'border-accentInfo');
                t.classList.add('text-textMuted');
            });
            tab.classList.add('active', 'text-accentInfo', 'border-accentInfo');
            tab.classList.remove('text-textMuted');
            
            const tabName = tab.dataset.tab;
            if (tabName === 'analysis') {
                analysisSection.classList.remove('hidden');
                analysisContent.classList.remove('hidden');
                chatContent.classList.add('hidden');
            } else {
                analysisSection.classList.add('hidden');
                analysisContent.classList.add('hidden');
                chatContent.classList.remove('hidden');
            }
        });
    });

    // Setup chat send button
    const sendBtn = document.getElementById('ai-chat-send');
    const chatInput = document.getElementById('ai-chat-input');
    
    if (sendBtn) {
        sendBtn.addEventListener('click', sendChatMessage);
    }
    if (chatInput) {
        chatInput.addEventListener('keypress', (e) => {
            if (e.key === 'Enter' && !e.shiftKey) {
                e.preventDefault();
                sendChatMessage();
            }
        });
    }
}

function setupSymbolInput() {
    const analyzeBtn = document.getElementById('ai-analyze-btn');
    const symbolInput = document.getElementById('ai-symbol-input');
    
    if (analyzeBtn) {
        analyzeBtn.addEventListener('click', () => {
            if (symbolInput && symbolInput.value.trim()) {
                analyzeSymbol(symbolInput.value.trim().toUpperCase());
            }
        });
    }
    
    if (symbolInput) {
        symbolInput.addEventListener('keypress', (e) => {
            if (e.key === 'Enter' && symbolInput.value.trim()) {
                analyzeSymbol(symbolInput.value.trim().toUpperCase());
            }
        });
    }
}

function setupQuickPickGrid() {
    document.querySelectorAll('.quick-symbol-btn').forEach(btn => {
        btn.addEventListener('click', () => {
            const symbol = btn.dataset.symbol;
            const input = document.getElementById('ai-symbol-input');
            if (input) input.value = symbol;
            analyzeSymbol(symbol);
        });
    });
}

export function openAIAnalysisModal(symbol = '') {
    const modal = document.getElementById('ai-analysis-modal');
    if (modal) {
        modal.classList.remove('hidden');
        document.body.style.overflow = 'hidden';
        
        if (symbol) {
            const input = document.getElementById('ai-symbol-input');
            if (input) {
                input.value = symbol;
                analyzeSymbol(symbol);
            }
        }
    }
}

export function closeAIAnalysisModal() {
    const modal = document.getElementById('ai-analysis-modal');
    if (modal) {
        modal.classList.add('hidden');
        document.body.style.overflow = '';
    }
    
    if (currentAnalysisStream) {
        currentAnalysisStream.abort();
        currentAnalysisStream = null;
    }
    
    isAnalyzing = false;
    currentSymbol = '';
}

async function analyzeSymbol(symbol) {
    if (isAnalyzing) {
        showToast('Analysis already in progress...', 'warning');
        return;
    }
    
    isAnalyzing = true;
    currentSymbol = symbol;
    const contentDiv = document.getElementById('ai-analysis-content');
    const analyzeBtn = document.getElementById('ai-analyze-btn');
    
    // Update sidebar with loading state
    updateSymbolStatsLoading(symbol);
    
    if (analyzeBtn) {
        analyzeBtn.disabled = true;
        analyzeBtn.innerHTML = `
            <svg class="animate-spin h-4 w-4" viewBox="0 0 24 24">
                <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4" fill="none"></circle>
                <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
            </svg>
            <span>Analyzing...</span>
        `;
    }
    
    if (contentDiv) {
        contentDiv.innerHTML = `
            <div class="flex flex-col items-center justify-center h-64">
                <div class="relative mb-6">
                    <div class="w-20 h-20 border-4 border-purple-500/30 border-t-purple-500 rounded-full animate-spin"></div>
                    <div class="absolute inset-0 flex items-center justify-center">
                        <span class="text-3xl">🤖</span>
                    </div>
                </div>
                <p class="text-textSecondary animate-pulse text-lg">Analyzing ${symbol}...</p>
                <p class="text-textMuted text-sm mt-2">Processing whale flows and market patterns</p>
            </div>
        `;
    }
    
    const renderer = new StreamingTextRenderer('ai-analysis-content');
    
    // Fetch candle data in parallel for sidebar
    fetchCandleDataForSidebar(symbol);
    
    currentAnalysisStream = createAISymbolAnalysisStream(symbol, {
        onMessage: (chunk, fullText) => {
            renderer.append(chunk);
        },
        onError: (err) => {
            console.error('AI Analysis error:', err);
            if (contentDiv) {
                contentDiv.innerHTML = `
                    <div class="flex flex-col items-center justify-center h-64 text-accentDanger">
                        <div class="text-6xl mb-4">⚠️</div>
                        <p class="text-lg font-semibold">Analysis Failed</p>
                        <p class="text-sm text-textMuted mt-2 text-center max-w-md">Unable to connect to AI service</p>
                        <p class="text-xs text-textMuted mt-1">Possible causes: No whale data, LLM disabled, or network issue</p>
                        <button onclick="analyzeSymbol('${symbol}')" class="mt-4 px-4 py-2 bg-accentInfo/20 text-accentInfo rounded-lg hover:bg-accentInfo/30 transition-colors">
                            Try Again
                        </button>
                    </div>
                `;
            }
            isAnalyzing = false;
            if (analyzeBtn) {
                analyzeBtn.disabled = false;
                analyzeBtn.innerHTML = `
                    <span>Analyze</span>
                    <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 10V3L4 14h7v7l9-11h-7z"></path>
                    </svg>
                `;
            }
        },
        onDone: (finalText) => {
            isAnalyzing = false;
            if (analyzeBtn) {
                analyzeBtn.disabled = false;
                analyzeBtn.innerHTML = `
                    <span>Analyze</span>
                    <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 10V3L4 14h7v7l9-11h-7z"></path>
                    </svg>
                `;
            }
            
            renderer.finalize();
            
            if (contentDiv && finalText) {
                const completionDiv = document.createElement('div');
                completionDiv.className = 'mt-6 pt-4 border-t border-borderColor';
                completionDiv.innerHTML = `
                    <p class="text-xs text-textMuted flex items-center gap-2">
                        <span class="w-2 h-2 rounded-full bg-accentSuccess animate-pulse"></span>
                        Analysis completed at ${new Date().toLocaleTimeString('id-ID')}
                    </p>
                `;
                contentDiv.appendChild(completionDiv);
            }
            
            showToast(`Analysis for ${symbol} completed!`, 'success');
        }
    });
}

async function fetchCandleDataForSidebar(symbol) {
    try {
        const data = await fetchCandles(symbol, '1h');
        if (data && data.candles && data.candles.length > 0) {
            updateSymbolStats(symbol, data);
        }
    } catch (error) {
        console.error('Failed to fetch candle data:', error);
    }
}

function updateSymbolStatsLoading(symbol) {
    const statsContainer = document.getElementById('symbol-stats');
    if (!statsContainer) return;
    
    statsContainer.innerHTML = `
        <div class="bg-bgSecondary rounded-xl p-4 border border-borderColor">
            <div class="flex items-center justify-between mb-4">
                <span class="text-lg font-bold text-accentInfo">${symbol}</span>
                <span class="text-xs text-textMuted animate-pulse">Loading...</span>
            </div>
            <div class="space-y-3">
                <div class="h-4 bg-bgPrimary rounded animate-pulse"></div>
                <div class="h-4 bg-bgPrimary rounded animate-pulse w-3/4"></div>
                <div class="h-4 bg-bgPrimary rounded animate-pulse w-1/2"></div>
            </div>
        </div>
    `;
}

function updateSymbolStats(symbol, data) {
    const statsContainer = document.getElementById('symbol-stats');
    if (!statsContainer || !data || !data.candles) return;
    
    const candles = data.candles;
    const latest = candles[candles.length - 1] || {};
    const first = candles[0] || {};
    
    const price = latest.close || 0;
    const openPrice = first.open || price;
    const priceChange = openPrice > 0 ? ((price - openPrice) / openPrice * 100) : 0;
    const changeClass = priceChange >= 0 ? 'text-accentSuccess' : 'text-accentDanger';
    const changeSign = priceChange >= 0 ? '+' : '';
    
    // Calculate volume ratio
    const avgVolume = candles.reduce((sum, c) => sum + (c.volume || 0), 0) / candles.length;
    const latestVolume = latest.volume || 0;
    const volumeRatio = avgVolume > 0 ? latestVolume / avgVolume : 1;
    
    // Calculate high/low
    const high = Math.max(...candles.map(c => c.high || 0));
    const low = Math.min(...candles.map(c => c.low || Infinity));
    low === Infinity && (low = 0);
    
    statsContainer.innerHTML = `
        <div class="bg-bgSecondary rounded-xl p-4 border border-borderColor">
            <div class="flex items-center justify-between mb-4">
                <span class="text-xl font-bold text-accentInfo">${symbol}</span>
                <span class="text-xs text-textMuted">1H Data</span>
            </div>
            
            <div class="text-center mb-4">
                <div class="text-3xl font-bold text-textPrimary">${formatNumber(price)}</div>
                <div class="text-lg font-semibold ${changeClass}">${changeSign}${priceChange.toFixed(2)}%</div>
            </div>
            
            <div class="grid grid-cols-2 gap-3 mb-4">
                <div class="bg-bgPrimary/50 rounded-lg p-2 text-center">
                    <div class="text-[10px] text-textMuted uppercase">High</div>
                    <div class="text-sm font-semibold text-accentSuccess">${formatNumber(high)}</div>
                </div>
                <div class="bg-bgPrimary/50 rounded-lg p-2 text-center">
                    <div class="text-[10px] text-textMuted uppercase">Low</div>
                    <div class="text-sm font-semibold text-accentDanger">${formatNumber(low)}</div>
                </div>
            </div>
            
            <div class="space-y-2">
                <div class="flex justify-between items-center text-xs">
                    <span class="text-textMuted">Volume Ratio</span>
                    <span class="font-semibold ${volumeRatio >= 1.5 ? 'text-accentSuccess' : volumeRatio >= 1 ? 'text-accentWarning' : 'text-textSecondary'}">
                        ${volumeRatio.toFixed(1)}x
                    </span>
                </div>
                <div class="flex justify-between items-center text-xs">
                    <span class="text-textMuted">Data Points</span>
                    <span class="font-semibold text-textPrimary">${candles.length}</span>
                </div>
                <div class="flex justify-between items-center text-xs">
                    <span class="text-textMuted">Time Range</span>
                    <span class="font-semibold text-textPrimary">${candles.length}h</span>
                </div>
            </div>
            
            <button onclick="analyzeSymbol('${symbol}')" class="w-full mt-4 px-3 py-2 bg-accentInfo/20 hover:bg-accentInfo/30 text-accentInfo rounded-lg text-sm font-medium transition-colors">
                🔄 Re-analyze
            </button>
        </div>
    `;
}

function formatAnalysisText(text) {
    if (!text) return '';
    
    const lines = text.split('\n');
    let result = [];
    let currentParagraph = [];
    let inList = false;
    let listItems = [];
    
    const flushParagraph = () => {
        if (currentParagraph.length > 0) {
            const content = currentParagraph.join(' ').trim();
            if (content) {
                result.push(`<p class="mb-4 text-textSecondary leading-relaxed">${content}</p>`);
            }
            currentParagraph = [];
        }
    };
    
    const flushList = () => {
        if (listItems.length > 0) {
            result.push('<ul class="mb-4 space-y-2 ml-4">' + listItems.join('') + '</ul>');
            listItems = [];
            inList = false;
        }
    };
    
    for (let i = 0; i < lines.length; i++) {
        let line = lines[i].trim();
        if (!line) {
            flushParagraph();
            flushList();
            continue;
        }
        
        if (line.match(/^#{1,3}\s/)) {
            flushParagraph();
            flushList();
            const level = line.match(/^(#+)/)[0].length;
            const content = line.replace(/^#+\s*/, '');
            const sizes = { 1: 'text-2xl', 2: 'text-xl', 3: 'text-lg' };
            result.push(`<h${level} class="${sizes[level]} font-bold text-accentInfo mt-6 mb-3">${formatInline(content)}</h${level}>`);
            continue;
        }
        
        const listMatch = line.match(/^(\d+\.\s+|[-*]\s+)(.+)/);
        if (listMatch) {
            flushParagraph();
            inList = true;
            const content = formatInline(listMatch[2]);
            listItems.push(`<li class="ml-2 text-textSecondary flex items-start gap-2"><span class="text-accentInfo mt-1">▸</span><span>${content}</span></li>`);
            continue;
        }
        
        if (inList && !line.match(/^(#{1,3}\s+|\d+\.\s+|[-*]\s+)/)) {
            if (listItems.length > 0) {
                const lastIdx = listItems.length - 1;
                listItems[lastIdx] = listItems[lastIdx].replace('</span></li>', ` ${formatInline(line)}</span></li>`);
            }
            continue;
        }
        
        flushList();
        currentParagraph.push(formatInline(line));
    }
    
    flushParagraph();
    flushList();
    
    return result.join('\n');
}

function formatInline(text) {
    return text
        .replace(/\*\*(.+?)\*\*/g, '<strong class="text-textPrimary">$1</strong>')
        .replace(/AGGRESSIVE BUY/g, '<span class="px-2 py-0.5 bg-accentSuccess/20 text-accentSuccess rounded text-sm font-bold">AGGRESSIVE BUY</span>')
        .replace(/ACCUMULATION/g, '<span class="px-2 py-0.5 bg-accentInfo/20 text-accentInfo rounded text-sm font-bold">ACCUMULATION</span>')
        .replace(/WAIT/g, '<span class="px-2 py-0.5 bg-yellow-500/20 text-yellow-400 rounded text-sm font-bold">WAIT</span>')
        .replace(/DISTRIBUTION/g, '<span class="px-2 py-0.5 bg-accentDanger/20 text-accentDanger rounded text-sm font-bold">DISTRIBUTION</span>')
        .replace(/BUY/g, '<span class="px-2 py-0.5 bg-accentSuccess/20 text-accentSuccess rounded text-sm font-bold">BUY</span>')
        .replace(/SELL/g, '<span class="px-2 py-0.5 bg-accentDanger/20 text-accentDanger rounded text-sm font-bold">SELL</span>')
        .replace(/\*(.+?)\*/g, '<em>$1</em>');
}

class StreamingTextRenderer {
    constructor(containerId) {
        this.container = document.getElementById(containerId);
        this.buffer = '';
        this.renderedContent = '';
        this.lastRenderTime = 0;
        this.renderDebounceMs = 50;
    }
    
    append(text) {
        this.buffer += text;
        this.scheduleRender();
    }
    
    scheduleRender() {
        const now = Date.now();
        const timeSinceLastRender = now - this.lastRenderTime;
        
        if (timeSinceLastRender >= this.renderDebounceMs) {
            this.render();
        } else {
            setTimeout(() => this.render(), this.renderDebounceMs - timeSinceLastRender);
        }
    }
    
    render() {
        if (!this.container) return;
        
        this.lastRenderTime = Date.now();
        const formatted = formatAnalysisText(this.buffer);
        
        if (formatted !== this.renderedContent) {
            this.renderedContent = formatted;
            this.container.innerHTML = formatted;
            this.container.scrollTop = this.container.scrollHeight;
        }
    }
    
    finalize() {
        this.render();
        this.buffer = '';
    }
}

function sendChatMessage() {
    const input = document.getElementById('ai-chat-input');
    const messagesDiv = document.getElementById('ai-chat-messages');
    
    if (!input || !messagesDiv) return;
    
    const message = input.value.trim();
    if (!message) return;
    
    addChatMessage('user', message);
    input.value = '';
    
    const messageId = 'ai-response-' + Date.now();
    addChatMessage('ai', '<div class="flex items-center gap-2"><div class="w-2 h-2 bg-purple-500 rounded-full animate-bounce"></div><div class="w-2 h-2 bg-purple-500 rounded-full animate-bounce" style="animation-delay: 0.1s"></div><div class="w-2 h-2 bg-purple-500 rounded-full animate-bounce" style="animation-delay: 0.2s"></div></div>', messageId);
    
    const messageDiv = document.getElementById(messageId);
    const contentDiv = messageDiv ? messageDiv.querySelector('.ai-message-content') : null;
    
    if (!contentDiv) return;
    
    let responseText = '';
    let lastUpdate = 0;
    const updateInterval = 100;
    
    currentPromptController = createAICustomPromptStream(message, {}, {
        onMessage: (chunk, fullText) => {
            responseText = fullText;
            const now = Date.now();
            if (now - lastUpdate > updateInterval) {
                lastUpdate = now;
                requestAnimationFrame(() => {
                    contentDiv.innerHTML = formatAnalysisText(responseText);
                    messagesDiv.scrollTop = messagesDiv.scrollHeight;
                });
            }
        },
        onError: (err) => {
            console.error('AI Chat error:', err);
            contentDiv.innerHTML = formatAnalysisText('Sorry, I encountered an error. Please try again.');
            contentDiv.classList.add('border', 'border-accentDanger');
        },
        onDone: (finalText) => {
            contentDiv.innerHTML = formatAnalysisText(finalText || responseText);
            messagesDiv.scrollTop = messagesDiv.scrollHeight;
        }
    });
}

function addChatMessage(type, content, id = null) {
    const messagesDiv = document.getElementById('ai-chat-messages');
    if (!messagesDiv) return;

    const messageDiv = document.createElement('div');
    messageDiv.className = 'flex gap-3';
    if (id) messageDiv.id = id;

    if (type === 'user') {
        messageDiv.innerHTML = `
            <div class="flex-1 flex justify-end">
                <div class="bg-accentInfo/20 text-textPrimary rounded-xl p-3 max-w-[80%]">
                    <p class="text-sm">${escapeHtml(content)}</p>
                </div>
            </div>
            <div class="w-10 h-10 rounded-xl bg-bgSecondary flex items-center justify-center flex-shrink-0">
                <span class="text-lg">👤</span>
            </div>
        `;
    } else {
        messageDiv.innerHTML = `
            <div class="w-10 h-10 rounded-xl bg-gradient-to-br from-purple-500 to-blue-500 flex items-center justify-center flex-shrink-0">
                <span class="text-lg">🤖</span>
            </div>
            <div class="flex-1">
                <div class="ai-message-content bg-bgSecondary rounded-xl p-4 text-textSecondary text-sm max-w-[90%] min-h-[40px]">
                    ${content}
                </div>
            </div>
        `;
    }

    messagesDiv.appendChild(messageDiv);
    messagesDiv.scrollTop = messagesDiv.scrollHeight;
}

function escapeHtml(text) {
    const div = document.createElement('div');
    div.textContent = text;
    return div.innerHTML;
}
