// Selection state for comparison
const selection = {
    profiles: new Map(), // id -> {type, name, created_at}
    lockedType: null,

    add(id, profile) {
        if (this.profiles.size === 0) {
            this.lockedType = profile.profile_type;
        }
        this.profiles.set(id, profile);
        this.updateUI();
    },

    remove(id) {
        this.profiles.delete(id);
        if (this.profiles.size === 0) {
            this.lockedType = null;
        }
        this.updateUI();
    },

    clear() {
        this.profiles.clear();
        this.lockedType = null;
        this.updateUI();
        document.querySelectorAll('.profile-checkbox').forEach(cb => {
            cb.checked = false;
            cb.disabled = false;
        });
    },

    has(id) {
        return this.profiles.has(id);
    },

    updateUI() {
        const bar = document.getElementById('compare-bar');
        const countEl = document.getElementById('compare-count');
        const btn = document.getElementById('compare-btn');
        if (!bar) return;

        const count = this.profiles.size;
        bar.hidden = count === 0;
        countEl.textContent = `${count} ${this.lockedType || ''} profile${count !== 1 ? 's' : ''} selected`;
        btn.disabled = count < 2;

        // Disable checkboxes of different types
        document.querySelectorAll('.profile-checkbox').forEach(cb => {
            const row = cb.closest('.profile-row');
            const type = row?.dataset.type;
            if (this.lockedType && type !== this.lockedType) {
                cb.disabled = true;
            } else {
                cb.disabled = false;
            }
        });
    }
};

// Simple client-side router with View Transitions
const router = {
    init() {
        window.addEventListener('popstate', () => this.route());
        document.addEventListener('click', e => {
            const link = e.target.closest('a[href^="/"]');
            if (link && !link.hasAttribute('download')) {
                e.preventDefault();
                this.navigate(link.getAttribute('href'));
            }
        });
        this.route();
    },

    navigate(path) {
        if (path === location.pathname) return;
        history.pushState(null, '', path);
        this.route();
    },

    async route() {
        const path = location.pathname;
        const main = document.getElementById('main-content');
        const headerProfile = document.getElementById('header-profile');

        const transition = async (callback) => {
            if (document.startViewTransition) {
                await document.startViewTransition(callback).finished;
            } else {
                await callback();
            }
        };

        if (path.startsWith('/compare/')) {
            const ids = path.split('/compare/')[1].split(',');
            headerProfile.hidden = true;
            await transition(() => this.renderCompare(main, ids));
        } else if (path.startsWith('/profile/')) {
            const id = path.split('/profile/')[1];
            headerProfile.hidden = false;
            await transition(() => this.renderProfile(main, id));
        } else {
            headerProfile.hidden = true;
            await transition(() => this.renderDashboard(main));
        }
    },

    renderDashboard(main) {
        const template = document.getElementById('dashboard-template');
        main.innerHTML = '';
        main.appendChild(template.content.cloneNode(true));
        loadProfiles();
    },

    async renderProfile(main, id) {
        const template = document.getElementById('profile-template');
        main.innerHTML = '';
        main.appendChild(template.content.cloneNode(true));
        await loadProfile(id);
    },

    async renderCompare(main, ids) {
        const template = document.getElementById('compare-template');
        main.innerHTML = '';
        main.appendChild(template.content.cloneNode(true));
        await loadCompare(ids);
    }
};

// Profile list
let refreshInterval;
let currentProject = '';

async function loadProfiles(project = currentProject) {
    clearInterval(refreshInterval);
    currentProject = project;

    try {
        const url = new URL('/api/profiles', location.origin);
        url.searchParams.set('limit', '50');
        if (project) url.searchParams.set('project', project);

        const response = await fetch(url);
        if (!response.ok) throw new Error('Failed to fetch');
        const profiles = await response.json();
        renderProfiles(profiles);
    } catch (err) {
        console.error('Failed to load profiles:', err);
    }

    refreshInterval = setInterval(() => loadProfiles(currentProject), 5000);
}

function setupProjectFilter(profiles) {
    const filter = document.getElementById('project-filter');
    if (!filter) return;

    // Extract unique projects
    const projects = [...new Set(profiles.map(p => p.project).filter(Boolean))].sort();

    // Preserve selection
    const current = filter.value || currentProject;

    // Populate options
    filter.innerHTML = '<option value="">All projects</option>';
    for (const proj of projects) {
        const opt = document.createElement('option');
        opt.value = proj;
        opt.textContent = proj;
        if (proj === current) opt.selected = true;
        filter.appendChild(opt);
    }

    // Add change handler (only once)
    if (!filter.dataset.listening) {
        filter.dataset.listening = 'true';
        filter.addEventListener('change', () => loadProfiles(filter.value));
    }
}

function renderProfiles(profiles) {
    const container = document.getElementById('profiles-container');
    if (!container) return;

    // Setup project filter dropdown
    setupProjectFilter(profiles);

    if (!profiles?.length) {
        container.innerHTML = `<div class="empty-state">No profiles collected yet</div>`;
        return;
    }

    // Sort profiles by date descending (newest first)
    profiles.sort((a, b) => new Date(b.created_at) - new Date(a.created_at));

    // Group by session
    const groups = new Map();
    for (const p of profiles) {
        const key = p.session || '';
        if (!groups.has(key)) groups.set(key, []);
        groups.get(key).push(p);
    }

    // Sort sessions by newest profile (descending)
    const sortedGroups = [...groups.entries()].sort((a, b) => {
        const aDate = new Date(a[1][0]?.created_at || 0);
        const bDate = new Date(b[1][0]?.created_at || 0);
        return bDate - aDate;
    });

    container.innerHTML = '';
    const sessionTpl = document.getElementById('session-template');
    const rowTpl = document.getElementById('profile-row-template');

    for (const [session, items] of sortedGroups) {
        let wrapper;
        if (session) {
            const details = sessionTpl.content.cloneNode(true);
            const header = details.querySelector('.session-header');
            const project = items[0]?.project || '';
            const date = formatDate(items[0]?.created_at);
            header.innerHTML = `
                <span class="session-name">${session}</span>
                <span class="session-meta">${project}${project && date ? ' · ' : ''}${date} · ${items.length} profiles</span>
            `;
            wrapper = details.querySelector('.session-profiles');
            container.appendChild(details);
        } else {
            wrapper = container;
        }

        for (const p of items) {
            const row = rowTpl.content.cloneNode(true);
            const rowEl = row.querySelector('.profile-row');
            rowEl.dataset.id = p.id;
            rowEl.dataset.type = p.profile_type;

            const checkbox = row.querySelector('.profile-checkbox');
            checkbox.checked = selection.has(p.id);
            if (selection.lockedType && p.profile_type !== selection.lockedType) {
                checkbox.disabled = true;
            }

            const nameLink = row.querySelector('.profile-name');
            nameLink.href = `/profile/${p.id}`;
            nameLink.textContent = p.name;

            row.querySelector('.profile-time').textContent = formatAbsoluteTime(p.created_at);
            const typeEl = row.querySelector('.profile-type');
            typeEl.textContent = p.profile_type;
            typeEl.classList.add(p.profile_type);
            row.querySelector('.profile-project').textContent = p.project || '—';
            row.querySelector('.profile-source').textContent = p.source || '—';
            row.querySelector('.profile-size').textContent = formatSize(p.raw_size);
            wrapper.appendChild(row);
        }
    }

    // Set up checkbox handlers
    container.addEventListener('change', e => {
        if (!e.target.classList.contains('profile-checkbox')) return;
        const row = e.target.closest('.profile-row');
        const id = row.dataset.id;
        const type = row.dataset.type;
        const name = row.querySelector('.profile-name').textContent;
        const time = row.querySelector('.profile-time').textContent;

        if (e.target.checked) {
            selection.add(id, { profile_type: type, name, created_at: time });
        } else {
            selection.remove(id);
        }
    });

    // Set up compare bar handlers
    const compareBtn = document.getElementById('compare-btn');
    const clearBtn = document.getElementById('clear-selection-btn');

    if (compareBtn) {
        compareBtn.onclick = () => {
            const ids = Array.from(selection.profiles.keys()).join(',');
            selection.clear();
            router.navigate(`/compare/${ids}`);
        };
    }

    if (clearBtn) {
        clearBtn.onclick = () => selection.clear();
    }

    // Update UI for any existing selection
    selection.updateUI();
}

// Profile detail
async function loadProfile(id) {
    try {
        const response = await fetch(`/api/profiles/${id}`);
        if (!response.ok) throw new Error('Profile not found');
        const profile = await response.json();
        renderProfile(profile);
    } catch (err) {
        console.error('Failed to load profile:', err);
        document.getElementById('profile-name').textContent = 'Profile not found';
    }
}

function renderProfile(profile) {
    // Header bar (in main header)
    document.getElementById('header-profile-name').textContent = profile.name;
    const typeEl = document.getElementById('header-profile-type');
    typeEl.textContent = profile.profile_type;
    typeEl.className = `tag ${profile.profile_type}`;
    document.getElementById('header-profile-project').textContent = profile.project || '';
    document.getElementById('header-profile-time').textContent = formatTime(profile.created_at);

    // Download link at bottom
    const downloadLink = document.getElementById('download-link');
    downloadLink.href = `/api/profiles/${profile.id}?raw=true`;
    // Update download link text based on profile type
    if (profile.profile_type === 'k6') {
        downloadLink.textContent = 'Download raw data (summary.json)';
    } else {
        downloadLink.textContent = 'Download raw profile (.pb.gz)';
    }

    // Optional metadata
    document.getElementById('profile-source').textContent = profile.source || '—';
    if (profile.session) {
        document.getElementById('session-item').hidden = false;
        document.getElementById('profile-session').textContent = profile.session;
    }

    // Type-specific metrics
    renderTypeMetrics(profile);

    // pprof commands (only for pprof profiles, not k6)
    const pprofCommandSection = document.querySelector('.pprof-command');
    if (profile.profile_type === 'k6') {
        // Hide pprof commands for k6 profiles
        pprofCommandSection.hidden = true;
    } else {
        // Show pprof commands for pprof profiles
        pprofCommandSection.hidden = false;
        const rawUrl = `${location.origin}/api/profiles/${profile.id}?raw=true`;
        const cliCmd = `go tool pprof ${rawUrl}`;
        const browserCmd = `go tool pprof -http=:8081 ${rawUrl}`;
        document.getElementById('pprof-cmd').innerHTML = `
            <div class="cmd-line">
                <span class="cmd-label">CLI</span>
                <code class="cmd-text">${cliCmd}</code>
                <button class="cmd-copy" onclick="copyToClipboard('${cliCmd}', this)">Copy</button>
            </div>
            <div class="cmd-line">
                <span class="cmd-label">Browser</span>
                <code class="cmd-text">${browserCmd}</code>
                <button class="cmd-copy" onclick="copyToClipboard('${browserCmd}', this)">Copy</button>
            </div>
        `;
    }

    const metricsJson = profile.metrics ? JSON.stringify(profile.metrics, null, 2) : 'No metrics available';
    document.getElementById('profile-metrics-data').textContent = metricsJson;
}

function renderTypeMetrics(profile) {
    const container = document.getElementById('type-metrics');
    const m = profile.metrics || {};
    let cards = [];
    let topItems = [];
    let topTitle = '';

    switch (profile.profile_type) {
        case 'cpu':
            cards = [
                { label: 'CPU Time', value: formatDuration(m.total_cpu_time_ns) },
                { label: 'Samples', value: formatNumber(m.sample_count) },
                { label: 'Size', value: formatSize(profile.raw_size) },
            ];
            topItems = m.top_functions || [];
            topTitle = 'Top Functions by CPU';
            break;

        case 'heap':
            cards = [
                { label: 'Alloc Size', value: formatBytes(m.alloc_size) },
                { label: 'Alloc Objects', value: formatNumber(m.alloc_objects) },
                { label: 'Inuse Size', value: formatBytes(m.inuse_size) },
                { label: 'Inuse Objects', value: formatNumber(m.inuse_objects) },
            ];
            topItems = m.top_allocators || [];
            topTitle = 'Top Allocators';
            break;

        case 'mutex':
            cards = [
                { label: 'Contention Time', value: formatDuration(m.contention_time_ns) },
                { label: 'Contentions', value: formatNumber(m.contention_count) },
                { label: 'Size', value: formatSize(profile.raw_size) },
            ];
            topItems = m.top_contenders || [];
            topTitle = 'Top Contenders';
            break;

        case 'block':
            cards = [
                { label: 'Blocking Time', value: formatDuration(m.blocking_time_ns) },
                { label: 'Block Events', value: formatNumber(m.blocking_count) },
                { label: 'Size', value: formatSize(profile.raw_size) },
            ];
            topItems = m.top_blockers || [];
            topTitle = 'Top Blockers';
            break;

        case 'goroutine':
            cards = [
                { label: 'Goroutines', value: formatNumber(m.goroutine_count) },
                { label: 'Size', value: formatSize(profile.raw_size) },
            ];
            topItems = (m.top_stacks || []).map(s => ({
                name: s.stack?.[0] || 'unknown',
                value: s.count,
                percent: 0
            }));
            topTitle = 'Top Stacks';
            break;

        case 'gc':
            cards = [
                { label: 'Total Pause', value: formatDuration(m.pause_time_total_ns) },
                { label: 'Pause Count', value: formatNumber(m.pause_count) },
                { label: 'Heap Goal', value: formatBytes(m.heap_goal) },
                { label: 'Last Pause', value: formatDuration(m.last_pause_ns) },
            ];
            break;

        case 'k6':
            cards = [
                { label: 'P50', value: `${m.p50_ms?.toFixed(1) || '—'}ms` },
                { label: 'P95', value: `${m.p95_ms?.toFixed(1) || '—'}ms` },
                { label: 'P99', value: `${m.p99_ms?.toFixed(1) || '—'}ms` },
                { label: 'RPS', value: m.rps?.toFixed(1) || '—' },
                { label: 'Error Rate', value: `${((m.error_rate || 0) * 100).toFixed(2)}%` },
                { label: 'Requests', value: formatNumber(m.total_requests) },
            ];
            break;

        default:
            cards = [
                { label: 'Samples', value: formatNumber(profile.total_samples) },
                { label: 'Total Value', value: formatNumber(profile.total_value) },
                { label: 'Size', value: formatSize(profile.raw_size) },
            ];
    }

    // Render metric cards
    container.innerHTML = cards.map(c => `
        <div class="metric-card">
            <span class="metric-value">${c.value}</span>
            <span class="metric-label">${c.label}</span>
        </div>
    `).join('');

    // Render top functions/allocators
    const topContainer = document.getElementById('top-functions');
    const listContainer = document.getElementById('functions-list');
    if (topItems.length > 0) {
        document.getElementById('top-functions-title').textContent = topTitle;
        listContainer.innerHTML = topItems.slice(0, 10).map(fn => `
            <div class="function-row">
                <span class="function-name">${fn.name}</span>
                <span class="function-value">${formatNumber(fn.value)}</span>
                <span class="function-percent">${fn.percent?.toFixed(1) || '—'}%</span>
            </div>
        `).join('');
        topContainer.hidden = false;
    }
}

// Formatters
function formatAbsoluteTime(iso) {
    const d = new Date(iso);
    const h = d.getHours().toString().padStart(2, '0');
    const m = d.getMinutes().toString().padStart(2, '0');
    const s = d.getSeconds().toString().padStart(2, '0');
    const ms = d.getMilliseconds().toString().padStart(3, '0');
    return `${h}:${m}:${s}.${ms}`;
}

function formatTime(iso) {
    const d = new Date(iso);
    const now = new Date();
    const diff = now - d;

    if (diff < 60_000) return 'just now';
    if (diff < 3600_000) return `${Math.floor(diff / 60_000)}m ago`;
    if (diff < 86400_000) return `${Math.floor(diff / 3600_000)}h ago`;

    return d.toLocaleDateString(undefined, {
        month: 'short',
        day: 'numeric',
        hour: '2-digit',
        minute: '2-digit'
    });
}

function formatFullTime(iso) {
    return new Date(iso).toLocaleString(undefined, {
        year: 'numeric',
        month: 'short',
        day: 'numeric',
        hour: '2-digit',
        minute: '2-digit',
        second: '2-digit'
    });
}

function formatDate(iso) {
    if (!iso) return '';
    return new Date(iso).toLocaleDateString(undefined, {
        month: 'short',
        day: 'numeric',
        year: 'numeric'
    });
}

function formatSize(bytes) {
    if (!bytes) return '—';
    if (bytes < 1024) return `${bytes} B`;
    if (bytes < 1024 ** 2) return `${(bytes / 1024).toFixed(1)} KB`;
    return `${(bytes / 1024 ** 2).toFixed(1)} MB`;
}

function formatBytes(bytes) {
    if (!bytes) return '—';
    if (bytes < 1024) return `${bytes} B`;
    if (bytes < 1024 ** 2) return `${(bytes / 1024).toFixed(1)} KB`;
    if (bytes < 1024 ** 3) return `${(bytes / 1024 ** 2).toFixed(1)} MB`;
    return `${(bytes / 1024 ** 3).toFixed(2)} GB`;
}

function formatNumber(n) {
    if (!n) return '—';
    return n.toLocaleString();
}

function formatDuration(ns) {
    if (!ns) return '—';
    if (ns < 1000) return `${ns}ns`;
    if (ns < 1_000_000) return `${(ns / 1000).toFixed(1)}µs`;
    if (ns < 1_000_000_000) return `${(ns / 1_000_000).toFixed(1)}ms`;
    return `${(ns / 1_000_000_000).toFixed(2)}s`;
}

// Copy to clipboard
async function copyToClipboard(text, btn) {
    try {
        await navigator.clipboard.writeText(text);
        const original = btn.textContent;
        btn.textContent = 'Copied!';
        btn.classList.add('copied');
        setTimeout(() => {
            btn.textContent = original;
            btn.classList.remove('copied');
        }, 1500);
    } catch (err) {
        console.error('Failed to copy:', err);
    }
}

// Comparison view
async function loadCompare(ids) {
    try {
        const response = await fetch(`/api/profiles/compare?ids=${ids.join(',')}`);
        if (!response.ok) throw new Error('Failed to fetch profiles');
        const profiles = await response.json();
        renderCompare(profiles);
    } catch (err) {
        console.error('Failed to load comparison:', err);
        document.getElementById('compare-content').innerHTML =
            '<div class="empty-state">Failed to load profiles for comparison</div>';
    }
}

function renderCompare(profiles) {
    if (!profiles?.length) {
        document.getElementById('compare-content').innerHTML =
            '<div class="empty-state">No profiles to compare</div>';
        return;
    }

    // Sort by created_at ascending (oldest first)
    profiles.sort((a, b) => new Date(a.created_at) - new Date(b.created_at));

    const profileType = profiles[0].profile_type;
    document.getElementById('compare-type-label').textContent = profileType;

    // Set up view toggle
    const viewToggle = document.querySelector('.compare-view-toggle');
    viewToggle?.addEventListener('click', e => {
        const btn = e.target.closest('.view-btn');
        if (!btn) return;
        viewToggle.querySelectorAll('.view-btn').forEach(b => b.classList.remove('active'));
        btn.classList.add('active');
        const view = btn.dataset.view;
        if (view === 'timeline') {
            renderTimelineView(profiles);
        } else {
            renderTableView(profiles);
        }
    });

    // Default to table view
    renderTableView(profiles);
}

function renderTableView(profiles) {
    const container = document.getElementById('compare-content');
    const metrics = getMetricsForType(profiles[0].profile_type);

    let html = '<div class="compare-table compare-table-transposed">';

    // Header row with metric names
    html += '<div class="compare-row compare-header-row">';
    html += '<div class="compare-cell compare-profile-col">Profile</div>';
    for (const metric of metrics) {
        html += `<div class="compare-cell compare-metric-header">${metric.label}</div>`;
    }
    html += '</div>';

    // Profile rows
    for (let i = 0; i < profiles.length; i++) {
        const p = profiles[i];
        const prev = i > 0 ? profiles[i - 1] : null;

        html += '<div class="compare-row">';
        html += `<div class="compare-cell compare-profile-col">
            <div class="compare-profile-name">${p.name}</div>
            <div class="compare-profile-time">${formatAbsoluteTime(p.created_at)}</div>
        </div>`;

        for (const metric of metrics) {
            const val = metric.getValue(p.metrics || {});
            const prevVal = prev ? metric.getValue(prev.metrics || {}) : null;
            const delta = prev ? calculateDelta(prevVal, val, metric) : null;

            html += `<div class="compare-cell">
                <span class="cell-value">${metric.format(val)}</span>
                ${delta ? `<span class="cell-delta ${delta.class}">${delta.text}</span>` : ''}
            </div>`;
        }

        html += '</div>';
    }

    html += '</div>';
    container.innerHTML = html;
}

function renderTimelineView(profiles) {
    const container = document.getElementById('compare-content');
    const metrics = getMetricsForType(profiles[0].profile_type);

    let html = '<div class="compare-timeline">';

    for (let i = 0; i < profiles.length; i++) {
        const p = profiles[i];
        const prev = i > 0 ? profiles[i - 1] : null;

        html += `<div class="timeline-entry">
            <div class="timeline-header">
                <span class="timeline-name">${p.name}</span>
                <span class="timeline-time">${formatFullTime(p.created_at)}</span>
            </div>
            <div class="timeline-metrics">`;

        for (const metric of metrics) {
            const val = metric.getValue(p.metrics || {});
            let deltaHtml = '';

            if (prev) {
                const prevVal = metric.getValue(prev.metrics || {});
                const delta = calculateDelta(prevVal, val, metric);
                deltaHtml = `<span class="timeline-delta ${delta.class}">${delta.text}</span>`;
            }

            html += `<div class="timeline-metric">
                <span class="timeline-metric-label">${metric.label}</span>
                <span class="timeline-metric-value">${metric.format(val)}</span>
                ${deltaHtml}
            </div>`;
        }

        html += '</div></div>';
    }

    html += '</div>';
    container.innerHTML = html;
}

function getMetricsForType(profileType) {
    const metricsConfig = {
        cpu: [
            { label: 'CPU Time', key: 'total_cpu_time_ns', format: formatDuration, lowerIsBetter: true },
            { label: 'Samples', key: 'sample_count', format: formatNumber, lowerIsBetter: false },
        ],
        heap: [
            { label: 'Alloc Size', key: 'alloc_size', format: formatBytes, lowerIsBetter: true },
            { label: 'Alloc Objects', key: 'alloc_objects', format: formatNumber, lowerIsBetter: true },
            { label: 'Inuse Size', key: 'inuse_size', format: formatBytes, lowerIsBetter: true },
            { label: 'Inuse Objects', key: 'inuse_objects', format: formatNumber, lowerIsBetter: true },
        ],
        mutex: [
            { label: 'Contention Time', key: 'contention_time_ns', format: formatDuration, lowerIsBetter: true },
            { label: 'Contentions', key: 'contention_count', format: formatNumber, lowerIsBetter: true },
        ],
        block: [
            { label: 'Blocking Time', key: 'blocking_time_ns', format: formatDuration, lowerIsBetter: true },
            { label: 'Block Events', key: 'blocking_count', format: formatNumber, lowerIsBetter: true },
        ],
        goroutine: [
            { label: 'Goroutines', key: 'goroutine_count', format: formatNumber, lowerIsBetter: true },
        ],
        gc: [
            { label: 'Total Pause', key: 'pause_time_total_ns', format: formatDuration, lowerIsBetter: true },
            { label: 'Pause Count', key: 'pause_count', format: formatNumber, lowerIsBetter: false },
            { label: 'Heap Goal', key: 'heap_goal', format: formatBytes, lowerIsBetter: false },
        ],
        k6: [
            { label: 'P50', key: 'p50_ms', format: v => v ? `${v.toFixed(1)}ms` : '—', lowerIsBetter: true },
            { label: 'P95', key: 'p95_ms', format: v => v ? `${v.toFixed(1)}ms` : '—', lowerIsBetter: true },
            { label: 'P99', key: 'p99_ms', format: v => v ? `${v.toFixed(1)}ms` : '—', lowerIsBetter: true },
            { label: 'RPS', key: 'rps', format: v => v ? v.toFixed(1) : '—', lowerIsBetter: false },
            { label: 'Error Rate', key: 'error_rate', format: v => `${((v || 0) * 100).toFixed(2)}%`, lowerIsBetter: true },
        ],
    };

    const config = metricsConfig[profileType] || [];
    return config.map(m => ({
        ...m,
        getValue: (metrics) => metrics[m.key],
    }));
}

function calculateDelta(oldVal, newVal, metric) {
    if (oldVal == null || newVal == null) {
        return { text: '—', class: '' };
    }

    const diff = newVal - oldVal;
    if (diff === 0) {
        return { text: '±0', class: 'delta-neutral' };
    }

    const pct = oldVal !== 0 ? ((diff / oldVal) * 100).toFixed(1) : '∞';
    const isIncrease = diff > 0;
    const isImproved = metric.lowerIsBetter ? !isIncrease : isIncrease;

    let text;
    if (typeof newVal === 'number' && newVal > 1000000) {
        text = `${isIncrease ? '+' : ''}${metric.format(diff)} (${isIncrease ? '+' : ''}${pct}%)`;
    } else {
        text = `${isIncrease ? '+' : ''}${pct}%`;
    }

    return {
        text,
        class: isImproved ? 'delta-improved' : 'delta-regressed',
    };
}

// Initialize
document.addEventListener('DOMContentLoaded', () => router.init());
