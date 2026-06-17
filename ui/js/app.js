// DOM Elements
const loginView = document.getElementById('login-view');
const appView = document.getElementById('app-view');
const loginForm = document.getElementById('login-form');
const loginError = document.getElementById('login-error');
const folderList = document.getElementById('folder-list');
const messageList = document.getElementById('message-list');
const searchInput = document.getElementById('search-input');
const currentFolderName = document.getElementById('current-folder-name');

// Reader Elements
const readerEmpty = document.getElementById('reader-empty');
const readerContent = document.getElementById('reader-content');
const readerSubject = document.getElementById('reader-subject');
const readerAvatar = document.getElementById('reader-avatar');
const readerFromName = document.getElementById('reader-from-name');
const readerFromEmail = document.getElementById('reader-from-email');
const readerDate = document.getElementById('reader-date');
const readerBody = document.getElementById('reader-body');

// State
let currentFolder = 'INBOX';
let activeMessageId = null;

// Initialize
async function init() {
    window.addEventListener('auth:unauthorized', showLogin);
    
    loginForm.addEventListener('submit', handleLogin);
    searchInput.addEventListener('input', debounce(handleSearch, 300));

    if (api.token) {
        try {
            const me = await api.getMe();
            if (me.is_admin) {
                document.getElementById('admin-btn').classList.remove('hidden');
            }
            showApp();
        } catch (e) {
            showLogin();
        }
    } else {
        showLogin();
    }
}

// ── Auth ──
function showLogin() {
    loginView.style.display = 'flex';
    appView.style.display = 'none';
    setTimeout(() => {
        loginView.style.opacity = '1';
    }, 10);
}

async function showApp() {
    loginView.style.opacity = '0';
    setTimeout(() => {
        loginView.style.display = 'none';
        appView.style.display = 'flex';
        loadInitialData();
    }, 500);
}

async function handleLogin(e) {
    e.preventDefault();
    const email = document.getElementById('email').value;
    const password = document.getElementById('password').value;
    const btn = document.getElementById('login-btn');
    
    try {
        btn.innerHTML = '<i data-feather="loader" class="spinner"></i>';
        feather.replace();
        btn.disabled = true;
        loginError.style.opacity = '0';

        await api.login(email, password);
        const me = await api.getMe();
        if (me.is_admin) {
            document.getElementById('admin-btn').classList.remove('hidden');
        }
        await showApp();
    } catch (err) {
        loginError.textContent = err.message;
        loginError.style.opacity = '1';
    } finally {
        btn.innerHTML = 'Sign In';
        btn.disabled = false;
    }
}

// ── Data Loading ──
async function loadInitialData() {
    try {
        await Promise.all([
            loadFolders(),
            loadMessages('INBOX')
        ]);
        setInterval(loadFolders, 30000); // refresh unread counts
    } catch (err) {
        console.error('Failed to load initial data', err);
    }
}

async function loadFolders() {
    const res = await api.getFolders();
    const foldersData = res.data || [];
    folderList.innerHTML = '';
    
    // Sort INBOX first, then Archive, Sent, Trash
    const order = ['INBOX', 'Archive', 'Sent', 'Drafts', 'Trash'];
    const sorted = foldersData.sort((a, b) => {
        const idxA = order.indexOf(a.name);
        const idxB = order.indexOf(b.name);
        if (idxA !== -1 && idxB !== -1) return idxA - idxB;
        if (idxA !== -1) return -1;
        if (idxB !== -1) return 1;
        return a.name.localeCompare(b.name);
    });

    sorted.forEach(f => {
        const li = document.createElement('li');
        li.className = `folder-item ${f.name === currentFolder ? 'active' : ''}`;
        li.onclick = () => selectFolder(f.name);

        let icon = 'folder';
        if (f.name === 'INBOX') icon = 'inbox';
        else if (f.name === 'Sent') icon = 'send';
        else if (f.name === 'Archive') icon = 'archive';
        else if (f.name === 'Trash') icon = 'trash-2';

        let badge = '';
        if (f.unread > 0) { // The struct json field is "unread" not "unread_count"
            badge = `<span class="unread-badge">${f.unread}</span>`;
        }

        li.innerHTML = `
            <div class="folder-icon">
                <i data-feather="${icon}" style="width: 18px; height: 18px;"></i>
                ${f.name}
            </div>
            ${badge}
        `;
        folderList.appendChild(li);
    });
    feather.replace();
}

async function selectFolder(folder) {
    currentFolder = folder;
    currentFolderName.textContent = folder;
    searchInput.value = '';
    await loadFolders(); // update active state
    await loadMessages(folder);
}

async function loadMessages(folder) {
    messageList.innerHTML = '<div style="padding: 2rem; text-align: center; color: var(--text-muted);"><i data-feather="loader" class="spinner"></i></div>';
    feather.replace();
    
    const res = await api.getMessages(folder);
    renderMessages(res.data || []);
}

async function handleSearch(e) {
    const q = e.target.value.trim();
    if (!q) {
        currentFolderName.textContent = currentFolder;
        return loadMessages(currentFolder);
    }
    
    currentFolderName.textContent = `Search: "${q}"`;
    messageList.innerHTML = '<div style="padding: 2rem; text-align: center; color: var(--text-muted);"><i data-feather="loader" class="spinner"></i></div>';
    feather.replace();
    
    const res = await api.searchMessages(q);
    renderMessages(res.data || []);
}

function renderMessages(messages) {
    messageList.innerHTML = '';
    
    if (!messages || messages.length === 0) {
        messageList.innerHTML = `
            <div style="padding: 3rem 2rem; text-align: center; color: var(--text-muted);">
                <i data-feather="inbox" style="width: 32px; height: 32px; margin-bottom: 1rem; opacity: 0.5;"></i>
                <p>No messages found.</p>
            </div>
        `;
        feather.replace();
        return;
    }

    messages.forEach(msg => {
        const div = document.createElement('div');
        div.className = `message-item ${msg.is_read ? 'read' : 'unread'} ${msg.id === activeMessageId ? 'active' : ''}`;
        div.onclick = () => openMessage(msg.id, div);

        // Parse "Name <email>" or just "email"
        let fromName = msg.from;
        if (msg.from.includes('<')) {
            fromName = msg.from.split('<')[0].trim();
        }

        const date = new Date(msg.date);
        const dateStr = formatDateFriendly(date);

        div.innerHTML = `
            <div class="msg-header">
                <span class="msg-from">${fromName}</span>
                <span class="msg-date">${dateStr}</span>
            </div>
            <div class="msg-subject">${msg.subject || '(No Subject)'}</div>
            <div class="msg-snippet">${msg.snippet || '...'}</div>
        `;
        messageList.appendChild(div);
    });
}

// ── Reading Pane ──
async function openMessage(id, element) {
    activeMessageId = id;
    
    // Update active UI state
    document.querySelectorAll('.message-item').forEach(el => el.classList.remove('active'));
    if (element) {
        element.classList.add('active');
        element.classList.remove('unread');
        element.classList.add('read');
    }

    readerEmpty.classList.add('hidden');
    readerContent.classList.remove('hidden');
    readerBody.innerHTML = '<div style="text-align:center; padding: 2rem;"><i data-feather="loader" class="spinner"></i></div>';
    feather.replace();

    try {
        const msg = await api.getMessage(id);
        
        // Mark as read in background if needed
        if (!msg.is_read) {
            api.updateMessage(id, { is_read: true }).then(() => loadFolders());
        }

        let fromName = msg.from;
        let fromEmail = msg.from;
        if (msg.from.includes('<')) {
            const parts = msg.from.split('<');
            fromName = parts[0].trim();
            fromEmail = parts[1].replace('>', '').trim();
        }

        readerSubject.textContent = msg.subject || '(No Subject)';
        readerFromName.textContent = fromName;
        readerFromEmail.textContent = fromEmail;
        readerAvatar.textContent = fromName.charAt(0).toUpperCase();
        readerDate.textContent = new Date(msg.date).toLocaleString();

        if (msg.body_html) {
            // Very simple sandboxing for MVP
            const iframe = document.createElement('iframe');
            iframe.style.width = '100%';
            iframe.style.height = '600px';
            iframe.style.border = 'none';
            iframe.style.background = 'white';
            iframe.style.borderRadius = '8px';
            readerBody.innerHTML = '';
            readerBody.appendChild(iframe);
            
            // Inject HTML into iframe safely
            const doc = iframe.contentWindow.document;
            doc.open();
            doc.write(msg.body_html);
            doc.close();
            
            // Adjust height to content
            setTimeout(() => {
                try {
                    iframe.style.height = doc.body.scrollHeight + 20 + 'px';
                } catch(e) {}
            }, 100);
        } else {
            const textContent = (msg.body_text || '').replace(/\n/g, '<br>');
            readerBody.innerHTML = `<div style="font-family: monospace; white-space: pre-wrap;">${textContent}</div>`;
        }
    } catch (err) {
        readerBody.innerHTML = `<div style="color: var(--danger)">Error loading message: ${err.message}</div>`;
    }
}

// ── Utils ──
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

function formatDateFriendly(date) {
    const now = new Date();
    const isToday = date.getDate() === now.getDate() && date.getMonth() === now.getMonth() && date.getFullYear() === now.getFullYear();
    
    if (isToday) {
        return date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
    }
    return date.toLocaleDateString([], { month: 'short', day: 'numeric' });
}

// Run!
init();
