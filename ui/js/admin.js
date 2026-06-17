// Admin Panel Logic

const adminBtn = document.getElementById('admin-btn');
const adminView = document.getElementById('admin-view');
const closeAdminBtn = document.getElementById('close-admin-btn');

const tabDomains = document.getElementById('tab-domains');
const tabAccounts = document.getElementById('tab-accounts');
const adminDomainsSec = document.getElementById('admin-domains-sec');
const adminAccountsSec = document.getElementById('admin-accounts-sec');

const domainsList = document.getElementById('domains-list');
const accountsList = document.getElementById('accounts-list');

// Setup event listeners if elements exist
if (adminBtn) {
    adminBtn.onclick = () => {
        adminView.classList.remove('hidden');
        loadAdminData();
    };

    closeAdminBtn.onclick = () => {
        adminView.classList.add('hidden');
    };

    tabDomains.onclick = () => {
        tabDomains.classList.add('active');
        tabAccounts.classList.remove('active');
        adminDomainsSec.classList.remove('hidden');
        adminAccountsSec.classList.add('hidden');
        loadAdminDomains();
    };

    tabAccounts.onclick = () => {
        tabAccounts.classList.add('active');
        tabDomains.classList.remove('active');
        adminAccountsSec.classList.remove('hidden');
        adminDomainsSec.classList.add('hidden');
        loadAdminAccounts();
    };

    document.getElementById('add-domain-btn').onclick = async () => {
        const name = prompt("Enter new domain name (e.g. skybeam.live):");
        if (name) {
            try {
                await api.request('/admin/domains', {
                    method: 'POST',
                    body: JSON.stringify({ name })
                });
                loadAdminDomains();
            } catch (err) {
                alert(err.message);
            }
        }
    };

    document.getElementById('add-account-btn').onclick = async () => {
        const email = prompt("Enter email address:");
        if (!email) return;
        const password = prompt("Enter password:");
        if (!password) return;
        const display_name = prompt("Enter display name:");
        const is_admin = confirm("Should this user be an admin?");
        
        try {
            await api.request('/admin/accounts', {
                method: 'POST',
                body: JSON.stringify({ email, password, display_name, is_admin })
            });
            loadAdminAccounts();
        } catch (err) {
            alert(err.message);
        }
    };
}

async function loadAdminData() {
    loadAdminDomains();
    loadAdminAccounts();
}

async function loadAdminDomains() {
    try {
        const res = await api.request('/admin/domains');
        const items = res.data || [];
        domainsList.innerHTML = items.map(d => `
            <div style="background: var(--bg-secondary); padding: 1rem; border-radius: 8px; display: flex; justify-content: space-between; align-items: center; border: 1px solid var(--border-color);">
                <div>
                    <strong>${d.name}</strong>
                    <div style="font-size: 0.8rem; color: var(--text-muted);">Added: ${new Date(d.created_at).toLocaleDateString()}</div>
                </div>
                <button class="icon-btn" onclick="deleteDomain('${d.id}')"><i data-feather="trash-2"></i></button>
            </div>
        `).join('');
        feather.replace();
    } catch (err) {
        console.error(err);
    }
}

async function loadAdminAccounts() {
    try {
        const res = await api.request('/admin/accounts');
        const items = res.data || [];
        accountsList.innerHTML = items.map(a => `
            <div style="background: var(--bg-secondary); padding: 1rem; border-radius: 8px; display: flex; justify-content: space-between; align-items: center; border: 1px solid var(--border-color);">
                <div>
                    <strong>${a.email}</strong> ${a.is_admin ? '<span style="background: var(--danger); color: white; padding: 2px 6px; border-radius: 4px; font-size: 0.7rem; margin-left: 8px;">ADMIN</span>' : ''}
                    <div style="font-size: 0.8rem; color: var(--text-muted);">${a.display_name || 'No Name'} • Created: ${new Date(a.created_at).toLocaleDateString()}</div>
                </div>
            </div>
        `).join('');
        feather.replace();
    } catch (err) {
        console.error(err);
    }
}

window.deleteDomain = async (id) => {
    if (confirm("Delete this domain?")) {
        await api.request(`/admin/domains/${id}`, { method: 'DELETE' });
        loadAdminDomains();
    }
};
