/**
 * AI Analysis Manager
 * Manages AI-powered symbol analysis and custom prompt interactions
 */

import { CONFIG } from './config.js';
import { safeGetElement, showToast } from './utils.js';
import { createAISymbolAnalysisStream, createAICustomPromptStream } from './api.js';

// State
let currentAnalysisStream = null;
let currentPromptController = null;
let isAnalyzing = false;

/**
 * Initialize AI Analysis system
 */
export function initAIAnalysis() {
    setupAIAnalysisModal();
    setupAIChatInterface();
    console.log('🤖 AI Analysis system initialized');
}

/**
 * Setup AI Analysis Modal
 */
function setupAIAnalysisModal() {
    // Check if modal exists, if not create it
    let modal = document.getElementById('ai-analysis-modal');
    if (!modal) {
        modal = createAIAnalysisModal();
        document.body.appendChild(modal);
    }

    // Close modal handlers
    const closeBtn = modal.querySelector('.ai-modal-close');
    const overlay = modal.querySelector('.ai-modal-overlay');
    
    if (closeBtn) {
        closeBtn.addEventListener('click', closeAIAnalysisModal);
    }
    if (overlay) {
        overlay.addEventListener('click', closeAIAnalysisModal);
    }

    // Setup analyze button
    const analyzeBtn = document.getElementById('ai-analyze-btn');
    if (analyzeBtn) {
        analyzeBtn.addEventListener('click', () => {
            const symbolInput = document.getElementById('ai-symbol-input');
            if (symbolInput && symbolInput.value.trim()) {
                analyzeSymbol(symbolInput.value.trim().toUpperCase());
            }
        });
    }

    // Enter key handler
    const symbolInput = document.getElementById('ai-symbol-input');
    if (symbolInput) {
        symbolInput.addEventListener('keypress', (e) => {
            if (e.key === 'Enter' && symbolInput.value.trim()) {
                analyzeSymbol(symbolInput.value.trim().toUpperCase());
            }
        });
    }
}

/**
 * Create AI Analysis Modal HTML
 */
function createAIAnalysisModal() {
    const modal = document.createElement('div');
    modal.id = 'ai-analysis-modal';
    modal.className = 'fixed inset-0 z-50 hidden';
    modal.innerHTML = `
        <div class="ai-modal-overlay absolute inset-0 bg-black/70 backdrop-blur-sm"></div>
        <div class="absolute inset-4 md:inset-10 bg-bgPrimary rounded-xl border border-borderColor shadow-2xl flex flex-col overflow-hidden">
            <!-- Header -->
            <div class="flex items-center justify-between p-4 border-b border-borderColor bg-bgSecondary">
                <div class="flex items-center gap-3">
                    <div class="w-10 h-10 rounded-lg bg-gradient-to-br from-purple-500 to-blue-500 flex items-center justify-center">
                        <span class="text-xl">🤖</span>
                    </div>
                    <div>
                        <h2 class="text-lg font-bold text-textPrimary">AI Market Analysis</h2>
                        <p class="text-xs text-textMuted">Powered by Quantum Trader AI</p>
                    </div>
                </div>
                <button class="ai-modal-close w-8 h-8 rounded-lg hover:bg-bgHover flex items-center justify-center text-textMuted hover:text-textPrimary transition-colors">
                    <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"></path>
                    </svg>
                </button>
            </div>
            
            <!-- Input Section -->
            <div class="p-4 border-b border-borderColor bg-bgSecondary/50">
                <div class="flex gap-2">
                    <div class="flex-1 relative">
                        <input 
                            type="text" 
                            id="ai-symbol-input" 
                            placeholder="Enter stock symbol (e.g., BBCA, TLKM, BBRI)..."
                            class="w-full px-4 py-3 bg-bgPrimary border border-borderColor rounded-lg text-textPrimary placeholder-textMuted focus:outline-none focus:border-accentInfo transition-colors"
                            maxlength="10"
                        >
                        <div class="absolute right-3 top-1/2 -translate-y-1/2 text-textMuted text-xs">
                            <kbd class="px-2 py-1 bg-bgSecondary rounded">ENTER</kbd>
                        </div>
                    </div>
                    <button 
                        id="ai-analyze-btn"
                        class="px-6 py-3 bg-gradient-to-r from-purple-600 to-blue-600 hover:from-purple-500 hover:to-blue-500 text-white font-semibold rounded-lg transition-all flex items-center gap-2 disabled:opacity-50 disabled:cursor-not-allowed"
                    >
                        <span>Analyze</span>
                        <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 10V3L4 14h7v7l9-11h-7z"></path>
                        </svg>
                    </button>
                </div>
            </div>
            
            <!-- Content Area -->
            <div class="flex-1 overflow-hidden flex flex-col md:flex-row">
                <!-- Analysis Result -->
                <div class="flex-1 overflow-y-auto p-4">
                    <div id="ai-analysis-content" class="prose prose-invert max-w-none">
                        <div class="flex flex-col items-center justify-center h-64 text-textMuted">
                            <div class="text-6xl mb-4">📊</div>
                            <p class="text-lg">Enter a stock symbol to get AI-powered analysis</p>
                            <p class="text-sm mt-2">The AI will analyze whale flows and patterns</p>
                        </div>
                    </div>
                </div>
                
                <!-- Quick Stats Sidebar -->
                <div class="w-full md:w-72 border-t md:border-t-0 md:border-l border-borderColor bg-bgSecondary/30 p-4 overflow-y-auto">
                    <h3 class="text-sm font-semibold text-textSecondary mb-3 uppercase tracking-wider">Quick Actions</h3>
                    <div class="space-y-2 mb-6">
                        <button onclick="analyzeQuickSymbol('BBCA')" class="w-full px-3 py-2 text-left text-sm bg-bgSecondary hover:bg-bgHover rounded-lg transition-colors text-textPrimary">
                            📈 Analyze BBCA
                        </button>
                        <button onclick="analyzeQuickSymbol('TLKM')" class="w-full px-3 py-2 text-left text-sm bg-bgSecondary hover:bg-bgHover rounded-lg transition-colors text-textPrimary">
                            📈 Analyze TLKM
                        </button>
                        <button onclick="analyzeQuickSymbol('BBRI')" class="w-full px-3 py-2 text-left text-sm bg-bgSecondary hover:bg-bgHover rounded-lg transition-colors text-textPrimary">
                            📈 Analyze BBRI
                        </button>
                        <button onclick="switchToAIChat()" class="w-full px-3 py-2 text-left text-sm bg-accentInfo/20 hover:bg-accentInfo/30 text-accentInfo rounded-lg transition-colors">
                            💬 Open AI Chat
                        </button>
                    </div>
                    
                    <h3 class="text-sm font-semibold text-textSecondary mb-3 uppercase tracking-wider">Analysis Includes</h3>
                    <ul class="space-y-2 text-xs text-textMuted">
                        <li class="flex items-center gap-2">
                            <span class="w-1.5 h-1.5 rounded-full bg-accentSuccess"></span>
                            Whale Flow Analysis
                        </li>
                        <li class="flex items-center gap-2">
                            <span class="w-1.5 h-1.5 rounded-full bg-accentInfo"></span>
                            Statistical Analysis
                        </li>
                        <li class="flex items-center gap-2">
                            <span class="w-1.5 h-1.5 rounded-full bg-accentWarning"></span>
                            Order Flow Imbalance
                        </li>
                        <li class="flex items-center gap-2">
                            <span class="w-1.5 h-1.5 rounded-full bg-purple-400"></span>
                            Historical Impact Data
                        </li>
                    </ul>
                </div>
            </div>
        </div>
    `;
    
    // Add global function for quick symbols
    window.analyzeQuickSymbol = (symbol) => {
        const input = document.getElementById('ai-symbol-input');
        if (input) {
            input.value = symbol;
            analyzeSymbol(symbol);
        }
    };
    
    return modal;
}

/**
 * Open AI Analysis Modal
 */
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
        } else {
            // Focus input
            setTimeout(() => {
                const input = document.getElementById('ai-symbol-input');
                if (input) input.focus();
            }, 100);
        }
    }
}

/**
 * Close AI Analysis Modal
 */
export function closeAIAnalysisModal() {
    const modal = document.getElementById('ai-analysis-modal');
    if (modal) {
        modal.classList.add('hidden');
        document.body.style.overflow = '';
    }
    
    // Cancel any ongoing analysis
    if (currentAnalysisStream) {
        currentAnalysisStream.close();
        currentAnalysisStream = null;
    }
    
    isAnalyzing = false;
}

/**
 * Analyze a symbol using AI
 */
function analyzeSymbol(symbol) {
    if (isAnalyzing) {
        showToast('Analysis already in progress...', 'warning');
        return;
    }
    
    isAnalyzing = true;
    const contentDiv = document.getElementById('ai-analysis-content');
    const analyzeBtn = document.getElementById('ai-analyze-btn');
    
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
                <div class="relative">
                    <div class="w-16 h-16 border-4 border-accentInfo/30 border-t-accentInfo rounded-full animate-spin"></div>
                    <div class="absolute inset-0 flex items-center justify-center">
                        <span class="text-2xl">🤖</span>
                    </div>
                </div>
                <p class="mt-4 text-textSecondary animate-pulse">Analyzing ${symbol}...</p>
                <p class="text-xs text-textMuted mt-2">Processing whale flows and patterns</p>
            </div>
        `;
    }
    
    let analysisText = '';
    
    currentAnalysisStream = createAISymbolAnalysisStream(symbol, {
        onMessage: (chunk, fullText) => {
            analysisText = fullText;
            if (contentDiv) {
                // Convert markdown-like formatting to HTML
                const formatted = formatAnalysisText(analysisText);
                contentDiv.innerHTML = formatted;
            }
        },
        onError: (err) => {
            console.error('AI Analysis error:', err);
            if (contentDiv) {
                contentDiv.innerHTML = `
                    <div class="flex flex-col items-center justify-center h-64 text-accentDanger">
                        <div class="text-5xl mb-4">⚠️</div>
                        <p class="text-lg font-semibold">Analysis Failed</p>
                        <p class="text-sm text-textMuted mt-2">${err.message || 'Unable to connect to AI service'}</p>
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
            
            if (contentDiv && finalText) {
                const formatted = formatAnalysisText(finalText);
                contentDiv.innerHTML = formatted + `
                    <div class="mt-6 pt-4 border-t border-borderColor">
                        <p class="text-xs text-textMuted flex items-center gap-2">
                            <span class="w-2 h-2 rounded-full bg-accentSuccess"></span>
                            Analysis completed at ${new Date().toLocaleTimeString('id-ID')}
                        </p>
                    </div>
                `;
            }
            
            showToast(`Analysis for ${symbol} completed!`, 'success');
        }
    });
}

/**
 * Format analysis text with HTML
 */
function formatAnalysisText(text) {
    if (!text) return '';
    
    // First, normalize the text
    let formatted = text
        // Headers (must be at start of line)
        .replace(/\*\*(.+?)\*\*/g, '<strong class="text-textPrimary">$1</strong>')
        .replace(/^(#{3}\s*)(.+)$/gm, '<h3 class="text-lg font-bold text-accentInfo mt-6 mb-3">$2</h3>')
        .replace(/^(#{2}\s*)(.+)$/gm, '<h2 class="text-xl font-bold text-accentInfo mt-6 mb-3">$2</h2>')
        .replace(/^(#{1}\s*)(.+)$/gm, '<h1 class="text-2xl font-bold text-accentInfo mt-6 mb-4">$2</h1>')
        // Signal badges
        .replace(/AGGRESSIVE BUY/g, '<span class="px-2 py-0.5 bg-accentSuccess/20 text-accentSuccess rounded text-sm font-bold">AGGRESSIVE BUY</span>')
        .replace(/ACCUMULATION/g, '<span class="px-2 py-0.5 bg-accentInfo/20 text-accentInfo rounded text-sm font-bold">ACCUMULATION</span>')
        .replace(/WAIT/g, '<span class="px-2 py-0.5 bg-yellow-500/20 text-yellow-400 rounded text-sm font-bold">WAIT</span>')
        .replace(/DISTRIBUTION/g, '<span class="px-2 py-0.5 bg-accentDanger/20 text-accentDanger rounded text-sm font-bold">DISTRIBUTION</span>');
    
    // Split by double newlines to create paragraphs
    const paragraphs = formatted.split(/\n\n+/);
    
    return paragraphs.map(para => {
        para = para.trim();
        if (!para) return '';
        
        // Skip if already wrapped in HTML tags
        if (para.startsWith('<h') || para.startsWith('<li') || para.startsWith('<p')) {
            return para;
        }
        
        // Check if it's a list item
        if (para.match(/^\d+\.\s/)) {
            return `<li class="ml-4 text-textSecondary mb-1">${para.replace(/^\d+\.\s/, '')}</li>`;
        }
        if (para.match(/^[-*]\s/)) {
            return `<li class="ml-4 text-textSecondary mb-1 flex items-start gap-2"><span class="text-accentInfo mt-1">•</span><span>${para.replace(/^[-*]\s/, '')}</span></li>`;
        }
        
        // Convert single newlines to spaces within a paragraph
        para = para.replace(/\n/g, ' ');
        
        // Wrap in paragraph
        return `<p class="mb-3 text-textSecondary leading-relaxed">${para}</p>`;
    }).filter(Boolean).join('\n');
}

/**
 * Setup AI Chat Interface
 */
function setupAIChatInterface() {
    // Check if chat modal exists, if not create it
    let chatModal = document.getElementById('ai-chat-modal');
    if (!chatModal) {
        chatModal = createAIChatModal();
        document.body.appendChild(chatModal);
    }

    // Close handlers
    const closeBtn = chatModal.querySelector('.ai-chat-close');
    const overlay = chatModal.querySelector('.ai-chat-overlay');
    
    if (closeBtn) {
        closeBtn.addEventListener('click', closeAIChatModal);
    }
    if (overlay) {
        overlay.addEventListener('click', closeAIChatModal);
    }

    // Send message handler
    const sendBtn = document.getElementById('ai-chat-send');
    const input = document.getElementById('ai-chat-input');
    
    if (sendBtn) {
        sendBtn.addEventListener('click', sendChatMessage);
    }
    
    if (input) {
        input.addEventListener('keypress', (e) => {
            if (e.key === 'Enter' && !e.shiftKey) {
                e.preventDefault();
                sendChatMessage();
            }
        });
    }
}

/**
 * Create AI Chat Modal
 */
function createAIChatModal() {
    const modal = document.createElement('div');
    modal.id = 'ai-chat-modal';
    modal.className = 'fixed inset-0 z-50 hidden';
    modal.innerHTML = `
        <div class="ai-chat-overlay absolute inset-0 bg-black/70 backdrop-blur-sm"></div>
        <div class="absolute inset-4 md:inset-20 bg-bgPrimary rounded-xl border border-borderColor shadow-2xl flex flex-col overflow-hidden">
            <!-- Header -->
            <div class="flex items-center justify-between p-4 border-b border-borderColor bg-bgSecondary">
                <div class="flex items-center gap-3">
                    <div class="w-10 h-10 rounded-lg bg-gradient-to-br from-green-500 to-blue-500 flex items-center justify-center">
                        <span class="text-xl">💬</span>
                    </div>
                    <div>
                        <h2 class="text-lg font-bold text-textPrimary">AI Trading Assistant</h2>
                        <p class="text-xs text-textMuted">Ask anything about market data</p>
                    </div>
                </div>
                <button class="ai-chat-close w-8 h-8 rounded-lg hover:bg-bgHover flex items-center justify-center text-textMuted hover:text-textPrimary transition-colors">
                    <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"></path>
                    </svg>
                </button>
            </div>
            
            <!-- Chat Messages -->
            <div id="ai-chat-messages" class="flex-1 overflow-y-auto p-4 space-y-4">
                <!-- Welcome Message -->
                <div class="flex gap-3">
                    <div class="w-8 h-8 rounded-lg bg-gradient-to-br from-purple-500 to-blue-500 flex items-center justify-center flex-shrink-0">
                        <span class="text-sm">🤖</span>
                    </div>
                    <div class="flex-1">
                        <div class="bg-bgSecondary rounded-lg p-3 text-textSecondary text-sm">
                            <p class="font-semibold text-textPrimary mb-1">AI Assistant Ready</p>
                            <p>I can help you analyze market data, whale flows, and trading patterns. Try asking:</p>
                            <ul class="mt-2 space-y-1 text-xs text-textMuted">
                                <li>• "Which stocks show strong accumulation today?"</li>
                                <li>• "What's the current trend for BBCA?"</li>
                                <li>• "Analyze buy/sell pressure in the last 4 hours"</li>
                            </ul>
                        </div>
                    </div>
                </div>
            </div>
            
            <!-- Input Area -->
            <div class="p-4 border-t border-borderColor bg-bgSecondary/50">
                <div class="flex gap-2">
                    <div class="flex-1 relative">
                        <textarea 
                            id="ai-chat-input" 
                            placeholder="Ask me anything about the market..."
                            class="w-full px-4 py-3 bg-bgPrimary border border-borderColor rounded-lg text-textPrimary placeholder-textMuted focus:outline-none focus:border-accentInfo transition-colors resize-none"
                            rows="2"
                            maxlength="500"
                        ></textarea>
                        <div class="absolute bottom-2 right-2 text-textMuted text-xs">
                            <kbd class="px-2 py-1 bg-bgSecondary rounded">Enter</kbd> to send
                        </div>
                    </div>
                    <button 
                        id="ai-chat-send"
                        class="px-4 py-3 bg-gradient-to-r from-green-600 to-blue-600 hover:from-green-500 hover:to-blue-500 text-white font-semibold rounded-lg transition-all flex items-center justify-center"
                    >
                        <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 19l9 2-9-18-9 18 9-2zm0 0v-8"></path>
                        </svg>
                    </button>
                </div>
            </div>
        </div>
    `;
    
    return modal;
}

/**
 * Open AI Chat Modal
 */
export function openAIChatModal() {
    const modal = document.getElementById('ai-chat-modal');
    if (modal) {
        modal.classList.remove('hidden');
        document.body.style.overflow = 'hidden';
        
        // Focus input
        setTimeout(() => {
            const input = document.getElementById('ai-chat-input');
            if (input) input.focus();
        }, 100);
    }
    
    // Also close analysis modal if open
    closeAIAnalysisModal();
}

/**
 * Close AI Chat Modal
 */
export function closeAIChatModal() {
    const modal = document.getElementById('ai-chat-modal');
    if (modal) {
        modal.classList.add('hidden');
        document.body.style.overflow = '';
    }
    
    // Cancel any ongoing prompt
    if (currentPromptController) {
        currentPromptController.abort();
        currentPromptController = null;
    }
}

/**
 * Send chat message
 */
function sendChatMessage() {
    const input = document.getElementById('ai-chat-input');
    const messagesDiv = document.getElementById('ai-chat-messages');
    
    if (!input || !messagesDiv) return;
    
    const message = input.value.trim();
    if (!message) return;
    
    // Add user message
    addChatMessage('user', message);
    input.value = '';
    
    // Add loading indicator
    const loadingId = 'loading-' + Date.now();
    addChatMessage('ai', '<div class="flex items-center gap-2"><div class="w-2 h-2 bg-accentInfo rounded-full animate-bounce"></div><div class="w-2 h-2 bg-accentInfo rounded-full animate-bounce" style="animation-delay: 0.1s"></div><div class="w-2 h-2 bg-accentInfo rounded-full animate-bounce" style="animation-delay: 0.2s"></div></div>', loadingId);
    
    // Send to AI
    let responseText = '';
    currentPromptController = createAICustomPromptStream(message, {}, {
        onMessage: (chunk, fullText) => {
            responseText = fullText;
            updateChatMessage(loadingId, responseText);
        },
        onError: (err) => {
            console.error('AI Chat error:', err);
            updateChatMessage(loadingId, 'Sorry, I encountered an error. Please try again.', true);
        },
        onDone: (finalText) => {
            // Message already updated, just ensure it's marked as complete
        }
    });
}

/**
 * Add chat message
 */
function addChatMessage(type, content, id = null) {
    const messagesDiv = document.getElementById('ai-chat-messages');
    if (!messagesDiv) return;
    
    const messageDiv = document.createElement('div');
    messageDiv.className = 'flex gap-3';
    if (id) messageDiv.id = id;
    
    if (type === 'user') {
        messageDiv.innerHTML = `
            <div class="flex-1 flex justify-end">
                <div class="bg-accentInfo/20 text-textPrimary rounded-lg p-3 max-w-[80%]">
                    <p class="text-sm">${escapeHtml(content)}</p>
                </div>
            </div>
            <div class="w-8 h-8 rounded-lg bg-bgSecondary flex items-center justify-center flex-shrink-0">
                <span class="text-sm">👤</span>
            </div>
        `;
    } else {
        messageDiv.innerHTML = `
            <div class="w-8 h-8 rounded-lg bg-gradient-to-br from-purple-500 to-blue-500 flex items-center justify-center flex-shrink-0">
                <span class="text-sm">🤖</span>
            </div>
            <div class="flex-1">
                <div class="bg-bgSecondary rounded-lg p-3 text-textSecondary text-sm max-w-[90%]">
                    ${content}
                </div>
            </div>
        `;
    }
    
    messagesDiv.appendChild(messageDiv);
    messagesDiv.scrollTop = messagesDiv.scrollHeight;
}

/**
 * Update chat message
 */
function updateChatMessage(id, content, isError = false) {
    const messageDiv = document.getElementById(id);
    if (!messageDiv) return;
    
    const contentDiv = messageDiv.querySelector('.bg-bgSecondary');
    if (contentDiv) {
        if (isError) {
            contentDiv.classList.add('border', 'border-accentDanger');
        }
        contentDiv.innerHTML = formatAnalysisText(content);
    }
    
    // Scroll to bottom
    const messagesDiv = document.getElementById('ai-chat-messages');
    if (messagesDiv) {
        messagesDiv.scrollTop = messagesDiv.scrollHeight;
    }
}

/**
 * Escape HTML
 */
function escapeHtml(text) {
    const div = document.createElement('div');
    div.textContent = text;
    return div.innerHTML;
}

// Global function to switch to AI chat
window.switchToAIChat = openAIChatModal;
