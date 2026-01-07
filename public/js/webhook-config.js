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
            modal.classList.add('show');
            loadWebhooks();
        });
    }

    if (closeBtn && modal) {
        closeBtn.addEventListener('click', () => {
            modal.classList.remove('show');
            resetWebhookForm();
        });
    }

    if (modal) {
        modal.addEventListener('click', (e) => {
            if (e.target === modal) {
                modal.classList.remove('show');
                resetWebhookForm();
            }
        });
    }

    if (addBtn) {
        addBtn.addEventListener('click', () => {
            resetWebhookForm();
            const formSection = safeGetElement('webhook-form-section');
            if (formSection) formSection.style.display = 'block';
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

    if (loading) loading.style.display = 'block';
    if (placeholder) placeholder.style.display = 'none';

    try {
        webhooks = await API.fetchWebhooks();
        renderWebhooks(webhooks, tbody, placeholder);
    } catch (error) {
        console.error('Failed to load webhooks:', error);
        if (tbody) {
            tbody.innerHTML = '<tr><td colspan="4" style="text-align: center; color: var(--accent-sell);">Failed to load webhooks</td></tr>';
        }
    } finally {
        if (loading) loading.style.display = 'none';
    }
}

/**
 * Render webhooks table
 */
function renderWebhooks(webhooks, tbody, placeholder) {
    if (!tbody) return;

    if (!webhooks || webhooks.length === 0) {
        tbody.innerHTML = '';
        if (placeholder) placeholder.style.display = 'block';
        return;
    }

    if (placeholder) placeholder.style.display = 'none';

    tbody.innerHTML = '';
    webhooks.forEach(webhook => {
        const row = document.createElement('tr');

        const statusClass = webhook.is_active ? 'diff-positive' : 'text-secondary';
        const statusText = webhook.is_active ? '✓ Active' : '✗ Disabled';

        row.innerHTML = `
            <td style="font-weight: 500;">${escapeHtml(webhook.name || 'Unnamed')}</td>
            <td style="font-size: 0.85em; word-break: break-all;">${escapeHtml(webhook.url || '')}</td>
            <td class="${statusClass}" style="font-weight: 500;">${statusText}</td>
            <td style="text-align: right;">
                <button class="btn-icon" onclick="window.editWebhook(${webhook.id})" title="Edit">
                    ✏️
                </button>
                <button class="btn-icon" onclick="window.deleteWebhook(${webhook.id})" title="Delete" style="color: var(--accent-sell);">
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
window.editWebhook = function (id) {
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
    if (formSection) formSection.style.display = 'block';
    if (formTitle) formTitle.textContent = 'Edit Webhook';
};

/**
 * Delete webhook
 */
window.deleteWebhook = async function (id) {
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
};

/**
 * Reset webhook form
 */
function resetWebhookForm() {
    editingWebhookId = null;

    const form = safeGetElement('webhook-form');
    const formSection = safeGetElement('webhook-form-section');
    const formTitle = safeGetElement('webhook-form-title');

    if (form) form.reset();
    if (formSection) formSection.style.display = 'none';
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
