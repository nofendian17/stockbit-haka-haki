/**
 * Webhook Configuration Module
 * Manages webhook CRUD operations and UI
 */

import * as API from './api.js';
import { safeGetElement } from './utils.js';

let webhooks = [];
let editingWebhookId = null;

/**
 * Initialize webhook management
 */
export function initWebhookManagement() {
    setupWebhookModal();
    setupWebhookForm();
    loadWebhooks();
}

/**
 * Setup webhook modal
 */
function setupWebhookModal() {
    const openBtn = safeGetElement('webhook-config-btn');
    const modal = safeGetElement('webhook-modal');
    const closeBtn = safeGetElement('webhook-modal-close');
    const addBtn = safeGetElement('add-webhook-btn');

    if (openBtn && modal) {
        openBtn.addEventListener('click', () => {
            modal.classList.remove('hidden');
            loadWebhooks();
        });
    }

    if (closeBtn && modal) {
        closeBtn.addEventListener('click', () => {
            modal.classList.add('hidden');
            resetWebhookForm();
        });
    }

    if (modal) {
        modal.addEventListener('click', (e) => {
            if (e.target === modal) {
                modal.classList.add('hidden');
                resetWebhookForm();
            }
        });
    }

    if (addBtn) {
        addBtn.addEventListener('click', () => {
            resetWebhookForm();
            const formSection = safeGetElement('webhook-form-section');
            if (formSection) formSection.classList.remove('hidden');
        });
    }
}

/**
 * Setup webhook form
 */
function setupWebhookForm() {
    const form = safeGetElement('webhook-form');
    const cancelBtn = safeGetElement('cancel-webhook-btn');

    if (form) {
        form.addEventListener('submit', async (e) => {
            e.preventDefault();
            await saveWebhook();
        });
    }

    if (cancelBtn) {
        cancelBtn.addEventListener('click', () => {
            resetWebhookForm();
        });
    }
}

/**
 * Load webhooks from API
 */
async function loadWebhooks() {
    const tbody = safeGetElement('webhook-list-body');
    const loading = safeGetElement('webhook-loading');
    const placeholder = safeGetElement('webhook-placeholder');

    if (loading) loading.classList.remove('hidden');
    if (placeholder) placeholder.classList.add('hidden');

    try {
        webhooks = await API.fetchWebhooks();
        renderWebhooks(webhooks, tbody, placeholder);
    } catch (error) {
        console.error('Failed to load webhooks:', error);
        if (tbody) {
            tbody.innerHTML = '<tr><td colspan="4" class="text-center text-accentDanger p-4">Failed to load webhooks</td></tr>';
        }
    } finally {
        if (loading) loading.classList.add('hidden');
    }
}

/**
 * Render webhooks table
 */
function renderWebhooks(webhooks, tbody, placeholder) {
    if (!tbody) return;

    if (!webhooks || webhooks.length === 0) {
        tbody.innerHTML = '';
        if (placeholder) placeholder.classList.remove('hidden');
        return;
    }

    if (placeholder) placeholder.classList.add('hidden');

    tbody.innerHTML = '';
    webhooks.forEach(webhook => {
        const row = document.createElement('tr');
        row.className = 'border-b border-borderColor last:border-0 hover:bg-bgHover transition-colors';

        const statusClass = webhook.is_active ? 'text-accentSuccess font-bold' : 'text-textMuted';
        const statusText = webhook.is_active ? '✓ Active' : '✗ Disabled';
        const toggleIcon = webhook.is_active ? '🔔' : '🔕';
        const toggleTitle = webhook.is_active ? 'Disable webhook' : 'Enable webhook';

        row.innerHTML = `
            <td data-label="Name" class="p-3 font-medium">${escapeHtml(webhook.name || 'Unnamed')}</td>
            <td data-label="URL" class="p-3 text-sm break-all">${escapeHtml(webhook.url || '')}</td>
            <td data-label="Status" class="p-3 ${statusClass}">${statusText}</td>
            <td data-label="Actions" class="p-3 text-right space-x-2">
                <button class="p-1 hover:text-accentInfo transition-colors" onclick="window.toggleWebhook(${webhook.id})" title="${toggleTitle}">
                    ${toggleIcon}
                </button>
                <button class="p-1 hover:text-accentWarning transition-colors" onclick="window.editWebhook(${webhook.id})" title="Edit">
                    ✏️
                </button>
                <button class="p-1 hover:text-accentDanger transition-colors" onclick="window.deleteWebhook(${webhook.id})" title="Delete">
                    🗑️
                </button>
            </td>
        `;

        tbody.appendChild(row);
    });
}

/**
 * Save webhook (create or update)
 */
async function saveWebhook() {
    const nameInput = document.getElementById('webhook-name');
    const urlInput = document.getElementById('webhook-url');
    const enabledInput = document.getElementById('webhook-enabled');
    const submitBtn = safeGetElement('save-webhook-btn');

    if (!nameInput || !urlInput || !enabledInput) return;

    const webhookData = {
        name: nameInput.value.trim(),
        url: urlInput.value.trim(),
        is_active: enabledInput.checked,
    };

    // Validation
    if (!webhookData.name) {
        alert('Please enter a webhook name');
        return;
    }

    if (!webhookData.url) {
        alert('Please enter a webhook URL');
        return;
    }

    if (!isValidUrl(webhookData.url)) {
        alert('Please enter a valid URL');
        return;
    }

    if (submitBtn) submitBtn.disabled = true;

    try {
        if (editingWebhookId) {
            await API.updateWebhook(editingWebhookId, webhookData);
            console.log('✅ Webhook updated successfully');
        } else {
            await API.createWebhook(webhookData);
            console.log('✅ Webhook created successfully');
        }

        resetWebhookForm();
        await loadWebhooks();
    } catch (error) {
        console.error('Failed to save webhook:', error);
        alert('Failed to save webhook. Please try again.');
    } finally {
        if (submitBtn) submitBtn.disabled = false;
    }
}

/**
 * Edit webhook
 */
export function editWebhook(id) {
    const webhook = webhooks.find(w => w.id === id);
    if (!webhook) return;

    editingWebhookId = id;

    const nameInput = document.getElementById('webhook-name');
    const urlInput = document.getElementById('webhook-url');
    const enabledInput = document.getElementById('webhook-enabled');
    const formSection = safeGetElement('webhook-form-section');
    const formTitle = safeGetElement('webhook-form-title');

    if (nameInput) nameInput.value = webhook.name || '';
    if (urlInput) urlInput.value = webhook.url || '';
    if (enabledInput) enabledInput.checked = webhook.is_active || false;
    if (formSection) formSection.classList.remove('hidden');
    if (formTitle) formTitle.textContent = 'Edit Webhook';
}

/**
 * Delete webhook
 */
export async function deleteWebhook(id) {
    const webhook = webhooks.find(w => w.id === id);
    if (!webhook) return;

    if (!confirm(`Are you sure you want to delete webhook "${webhook.name}"?`)) {
        return;
    }

    try {
        await API.deleteWebhook(id);
        console.log('✅ Webhook deleted successfully');
        await loadWebhooks();
    } catch (error) {
        console.error('Failed to delete webhook:', error);
        alert('Failed to delete webhook. Please try again.');
    }
}

/**
 * Toggle webhook active status
 */
export async function toggleWebhook(id) {
    const webhook = webhooks.find(w => w.id === id);
    if (!webhook) return;

    const newStatus = !webhook.is_active;
    
    try {
        await API.updateWebhook(id, {
            name: webhook.name,
            url: webhook.url,
            is_active: newStatus
        });
        console.log(`✅ Webhook ${newStatus ? 'enabled' : 'disabled'} successfully`);
        await loadWebhooks();
    } catch (error) {
        console.error('Failed to toggle webhook:', error);
        alert('Failed to toggle webhook. Please try again.');
    }
}

// Expose functions to global scope for inline onclick handlers
window.editWebhook = editWebhook;
window.deleteWebhook = deleteWebhook;
window.toggleWebhook = toggleWebhook;

/**
 * Reset webhook form
 */
function resetWebhookForm() {
    editingWebhookId = null;

    const form = safeGetElement('webhook-form');
    const formSection = safeGetElement('webhook-form-section');
    const formTitle = safeGetElement('webhook-form-title');

    if (form) form.reset();
    if (formSection) formSection.classList.add('hidden');
    if (formTitle) formTitle.textContent = 'Add New Webhook';
}

/**
 * Validate URL
 */
function isValidUrl(string) {
    try {
        const url = new URL(string);
        return url.protocol === 'http:' || url.protocol === 'https:';
    } catch (_) {
        return false;
    }
}

/**
 * Escape HTML to prevent XSS
 */
function escapeHtml(text) {
    const div = document.createElement('div');
    div.textContent = text;
    return div.innerHTML;
}
