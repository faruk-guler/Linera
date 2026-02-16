// ===== Search / Filter =====
function filterFiles() {
    var input, filter, table, tr, td, i, txtValue;
    input = document.getElementById("search-input");
    filter = input.value.trim().toUpperCase();
    table = document.getElementById("file-table-body");
    tr = table.getElementsByClassName("file-row");

    // If search is empty, show all files
    if (!filter) {
        for (i = 0; i < tr.length; i++) {
            tr[i].style.display = "";
        }
        return;
    }

    for (i = 0; i < tr.length; i++) {
        td = tr[i].getElementsByTagName("td")[1];
        if (td) {
            var nameEl = td.querySelector(".file-name") || td.querySelector("a");
            txtValue = nameEl ? (nameEl.textContent || nameEl.innerText).trim() : (td.textContent || td.innerText).trim();
            if (txtValue.toUpperCase().indexOf(filter) > -1) {
                tr[i].style.display = "";
            } else {
                tr[i].style.display = "none";
            }
        }
    }
}

// ===== Global Search =====
function globalSearch() {
    const input = document.getElementById('search-input');
    const query = input.value.trim();

    if (query.length < 2) {
        alert('Please enter at least 2 characters to search');
        return;
    }

    const resultsContainer = document.getElementById('search-results-container');
    const resultsBody = document.getElementById('search-results-body');
    const resultsTitle = document.getElementById('search-results-title');
    const fileListContainer = document.querySelector('.file-list-container');

    // Show loading
    resultsBody.innerHTML = '<div style="padding: 20px; text-align: center; color: var(--text-secondary);">🔍 Searching...</div>';
    resultsContainer.style.display = 'block';
    fileListContainer.style.display = 'none';

    fetch('/search?q=' + encodeURIComponent(query))
        .then(response => response.json())
        .then(data => {
            const results = data.results || [];
            const limitReached = data.limit_reached || false;

            let titleText = `Search Results for "${query}" (${results.length} found)`;
            if (limitReached) {
                titleText += " (Limit Reached)";
            }
            resultsTitle.textContent = titleText;

            if (results.length === 0) {
                resultsBody.innerHTML = '<div style="padding: 40px; text-align: center; color: var(--text-secondary);"><p style="font-size: 2rem;">📭</p><p>No files found</p></div>';
                return;
            }

            let html = '<table class="file-list"><thead><tr><th>Name</th><th>Location</th><th>Size</th><th>Actions</th></tr></thead><tbody>';
            results.forEach(file => {
                const icon = file.is_dir ? '📁' : '📄';
                const downloadLink = '/download/' + file.path;
                html += `<tr class="file-row">
                    <td><span class="icon">${icon}</span> ${file.name}</td>
                    <td style="color: var(--text-secondary); font-size: 0.85rem;">${file.path}</td>
                    <td>${file.size}</td>
                    <td>
                        ${file.is_dir ? '' : `
                            <a href="#" class="btn-download" onclick="previewFile('${file.path}', '${file.name}'); return false;">
                                <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"></circle><line x1="12" y1="8" x2="12" y2="16"></line><line x1="8" y1="12" x2="16" y2="12"></line></svg>
                                Preview
                            </a>
                            <a href="${downloadLink}" class="btn-download" download>
                                <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"></path><polyline points="7 10 12 15 17 10"></polyline><line x1="12" y1="15" x2="12" y2="3"></line></svg>
                                Download
                            </a>
                        `}
                    </td>
                </tr>`;
            });
            html += '</tbody></table>';

            if (limitReached) {
                html += `
                <div style="
                    margin-top: 15px;
                    padding: 12px;
                    background-color: rgba(255, 193, 7, 0.15);
                    border: 1px solid rgba(255, 193, 7, 0.3);
                    border-radius: 6px;
                    color: var(--text-primary);
                    text-align: center;
                    font-size: 0.9rem;
                    display: flex;
                    align-items: center;
                    justify-content: center;
                    gap: 8px;
                ">
                    <svg xmlns="http://www.w3.org/2000/svg" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="#ffc107" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z"></path><line x1="12" y1="9" x2="12" y2="13"></line><line x1="12" y1="17" x2="12.01" y2="17"></line></svg>
                    <span>Maximum searchable file limit (1000) reached. Showing first 1000 results.</span>
                </div>`;
            }

            resultsBody.innerHTML = html;
        })
        .catch(err => {
            resultsBody.innerHTML = '<div style="padding: 20px; text-align: center; color: var(--danger);">Error: ' + err + '</div>';
        });
}

function exitSearch() {
    document.getElementById('search-results-container').style.display = 'none';
    document.querySelector('.file-list-container').style.display = 'block';
}

// ===== Preview Modal =====
function previewFile(path, name) {
    const ext = name.split('.').pop().toLowerCase();
    const modal = document.getElementById('preview-modal');
    const title = document.getElementById('preview-title');
    const body = document.getElementById('preview-body');
    const dlBtn = document.getElementById('preview-download-btn');

    const imageExts = ['jpg', 'jpeg', 'png', 'gif', 'webp', 'svg', 'bmp', 'ico'];
    const textExts = ['txt', 'md', 'json', 'xml', 'log', 'css', 'js', 'html', 'go', 'c', 'cpp', 'h', 'java', 'py', 'sh', 'bat', 'cmd', 'ps1'];
    const videoExts = ['mp4', 'webm', 'ogg', 'mov'];
    const audioExts = ['mp3', 'wav', 'ogg'];
    const pdfExts = ['pdf'];

    const fileUrl = '/download/' + path;

    title.innerText = name;
    dlBtn.href = fileUrl;
    body.innerHTML = '<div style="padding: 20px; color: var(--text-secondary);">Loading...</div>';
    modal.style.display = 'block';

    if (imageExts.includes(ext)) {
        body.innerHTML = `<img src="${fileUrl}">`;
    } else if (textExts.includes(ext)) {
        fetch(fileUrl)
            .then(response => {
                const len = parseInt(response.headers.get('Content-Length') || '0', 10);
                if (len > 1024 * 1024) return "File too large to preview (>1MB).";
                return response.text();
            })
            .then(text => {
                body.innerHTML = `<pre>${escapeHtml(text)}</pre>`;
            })
            .catch(err => {
                body.innerHTML = `<div style="color: red;">Error loading preview: ${err}</div>`;
            });
    } else if (videoExts.includes(ext)) {
        body.innerHTML = `<video controls><source src="${fileUrl}" type="video/${ext === 'mov' ? 'mp4' : ext}">Your browser does not support video.</video>`;
    } else if (audioExts.includes(ext)) {
        body.innerHTML = `<audio controls style="width: 90%;"><source src="${fileUrl}" type="audio/${ext}">Your browser does not support audio.</audio>`;
    } else if (pdfExts.includes(ext)) {
        body.innerHTML = `<iframe src="${fileUrl}"></iframe>`;
    } else {
        body.innerHTML = `<div style="text-align: center; color: var(--text-secondary);">
            <p style="font-size: 2.5rem; margin-bottom: 8px;">📄</p>
            <p>Preview not available for this file type.</p>
        </div>`;
    }
}

function closePreview() {
    const modal = document.getElementById('preview-modal');
    const body = document.getElementById('preview-body');
    modal.style.display = 'none';
    body.innerHTML = '';
}

function escapeHtml(text) {
    return text.replace(/&/g, "&amp;")
        .replace(/</g, "&lt;")
        .replace(/>/g, "&gt;")
        .replace(/"/g, "&quot;")
        .replace(/'/g, "&#039;");
}

function confirmLogout(event) {
    event.preventDefault();
    if (window.confirm("Are you sure you want to log out?")) {
        window.location.href = "/logout";
    }
}

// ===== Heartbeat =====
setInterval(() => {
    fetch('/ping').catch(() => { });
}, 5000);

// ===== Dark Mode =====
(function initTheme() {
    const themeToggle = document.getElementById('theme-toggle');
    const bodyObj = document.body;
    const sunIcon = document.getElementById('sun-icon');
    const moonIcon = document.getElementById('moon-icon');

    const savedTheme = localStorage.getItem('theme');
    if (savedTheme) {
        bodyObj.classList.add(savedTheme);
        updateIcons(savedTheme === 'dark-mode');
    }

    themeToggle.addEventListener('click', () => {
        bodyObj.classList.toggle('dark-mode');
        const isDark = bodyObj.classList.contains('dark-mode');
        localStorage.setItem('theme', isDark ? 'dark-mode' : '');
        updateIcons(isDark);
    });

    function updateIcons(isDark) {
        if (isDark) {
            sunIcon.style.display = 'none';
            moonIcon.style.display = 'block';
        } else {
            sunIcon.style.display = 'block';
            moonIcon.style.display = 'none';
        }
    }
})();

// ===== Upload =====
(function initUpload() {
    const uploadSection = document.getElementById('upload-section');
    if (!uploadSection) return;

    const btnDoUpload = document.getElementById('btn-do-upload');
    const uploadStatus = document.getElementById('upload-status');
    const progressContainer = document.getElementById('progress-container');
    const progressBar = document.getElementById('progress-bar');

    // Read current path from data attribute injected by Go template
    const currentPath = uploadSection.getAttribute('data-path') || '';

    btnDoUpload.addEventListener('click', () => {
        const fileInput = document.getElementById('upload-file');
        const pinInput = document.getElementById('upload-pin');

        if (!fileInput.files.length) {
            showStatus('Please select a file', 'error');
            return;
        }
        if (!pinInput.value) {
            showStatus('Please enter the Upload PIN', 'error');
            return;
        }

        const formData = new FormData();
        formData.append('file', fileInput.files[0]);
        formData.append('pin', pinInput.value);
        formData.append('path', currentPath);

        btnDoUpload.disabled = true;
        progressContainer.style.display = 'block';
        uploadStatus.style.display = 'none';

        const xhr = new XMLHttpRequest();
        xhr.open('POST', '/upload', true);
        xhr.setRequestHeader('X-Upload-PIN', pinInput.value);

        xhr.upload.onprogress = (e) => {
            if (e.lengthComputable) {
                const percent = Math.round((e.loaded / e.total) * 100);
                progressBar.style.width = percent + '%';
            }
        };

        xhr.onload = () => {
            btnDoUpload.disabled = false;
            if (xhr.status === 200) {
                showStatus('Success! File uploaded.', 'success');
                fileInput.value = '';
                setTimeout(() => {
                    location.reload();
                }, 1500);
            } else {
                showStatus(xhr.responseText || 'Upload failed', 'error');
            }
        };

        xhr.onerror = () => {
            btnDoUpload.disabled = false;
            showStatus('Network error occurred', 'error');
        };

        xhr.send(formData);
    });

    function showStatus(msg, type) {
        uploadStatus.textContent = msg;
        uploadStatus.style.display = 'block';
        uploadStatus.style.background = type === 'success' ? 'rgba(40, 167, 69, 0.1)' : 'rgba(239, 68, 68, 0.1)';
        uploadStatus.style.color = type === 'success' ? '#28a745' : '#ef4444';
    }
})();

// ===== Bulk Selection =====
function toggleSelectAll() {
    const master = document.getElementById('select-all');
    const checkboxes = document.querySelectorAll('.file-checkbox');
    checkboxes.forEach(cb => cb.checked = master.checked);
    updateBulkActionState();
}

function updateBulkActionState() {
    const checkboxes = document.querySelectorAll('.file-checkbox:checked');
    const bulkBar = document.getElementById('bulk-action-bar');
    const bulkCount = document.getElementById('bulk-count');

    if (checkboxes.length > 0) {
        bulkBar.classList.add('active');
        bulkCount.textContent = checkboxes.length + ' items selected';
    } else {
        bulkBar.classList.remove('active');
    }
}

function clearSelection() {
    document.getElementById('select-all').checked = false;
    toggleSelectAll();
}

function downloadSelected() {
    const checkboxes = document.querySelectorAll('.file-checkbox:checked');
    const paths = Array.from(checkboxes).map(cb => cb.getAttribute('data-path'));

    if (paths.length === 0) return;

    const form = document.createElement('form');
    form.method = 'POST';
    form.action = '/download-bulk';

    const input = document.createElement('input');
    input.type = 'hidden';
    input.name = 'paths';
    input.value = paths.join('|');

    form.appendChild(input);
    document.body.appendChild(form);
    form.submit();
    document.body.removeChild(form);
}
