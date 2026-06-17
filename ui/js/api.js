const API_URL = 'http://localhost:8080/api/v1';

class API {
    constructor() {
        this.token = localStorage.getItem('skybeam_token');
    }

    setToken(token) {
        this.token = token;
        if (token) {
            localStorage.setItem('skybeam_token', token);
        } else {
            localStorage.removeItem('skybeam_token');
        }
    }

    async request(endpoint, options = {}) {
        const headers = {
            'Content-Type': 'application/json',
            ...(options.headers || {})
        };

        if (this.token) {
            headers['Authorization'] = `Bearer ${this.token}`;
        }

        const config = {
            ...options,
            headers,
        };

        try {
            const response = await fetch(`${API_URL}${endpoint}`, config);
            const data = await response.json();

            if (!response.ok) {
                if (response.status === 401) {
                    this.setToken(null);
                    window.dispatchEvent(new Event('auth:unauthorized'));
                }
                throw new Error(data.error || 'API Request Failed');
            }

            return data;
        } catch (error) {
            console.error(`API Error (${endpoint}):`, error);
            throw error;
        }
    }

    // Auth
    async login(email, password) {
        const data = await this.request('/auth/login', {
            method: 'POST',
            body: JSON.stringify({ email, password })
        });
        this.setToken(data.token);
        return data;
    }

    async getMe() {
        return this.request('/auth/me');
    }

    async logout() {
        await this.request('/auth/logout', { method: 'DELETE' });
        this.setToken(null);
    }

    // Folders
    async getFolders() {
        return this.request('/folders');
    }

    // Messages
    async getMessages(folder = 'INBOX', limit = 50, offset = 0) {
        return this.request(`/messages?folder=${folder}&limit=${limit}&offset=${offset}`);
    }

    async getMessage(id) {
        return this.request(`/messages/${id}`);
    }

    async searchMessages(query) {
        return this.request(`/search?q=${encodeURIComponent(query)}`);
    }

    async updateMessage(id, updates) {
        return this.request(`/messages/${id}`, {
            method: 'PATCH',
            body: JSON.stringify(updates)
        });
    }

    async deleteMessage(id) {
        return this.request(`/messages/${id}`, { method: 'DELETE' });
    }
}

window.api = new API();
