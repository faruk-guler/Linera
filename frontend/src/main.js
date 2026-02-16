import './style.css';
import {
    SelectFolder, StartServer, StopServer, GetConfig, SetPort,
    OpenInBrowser, SetUploadEnabled,
    RegenerateSessionPIN, RegenerateUploadPIN, GetQRCode, GetAboutInfo,
    ShowError
} from '../wailsjs/go/main/App';

const folderInput = document.getElementById('shared-folder');
const portInput = document.getElementById('port');

// Disable arrow keys for port input
portInput.addEventListener('keydown', function (e) {
    if (e.key === 'ArrowUp' || e.key === 'ArrowDown') {
        e.preventDefault();
    }
});
const uploadSwitch = document.getElementById('upload-enabled');
const statusDot = document.getElementById('status-dot');
const statusText = document.getElementById('status-text');
const startBtn = document.getElementById('start-btn');
const infoSection = document.getElementById('info-section');
const sessionPin = document.getElementById('session-pin');
const uploadPin = document.getElementById('upload-pin');
const networkLink = document.getElementById('network-link');
const localLink = document.getElementById('local-link');

let isRunning = false;

async function updateUI() {
    const config = await GetConfig();
    folderInput.value = config.sharedFolder || "";
    if (document.activeElement !== portInput) {
        portInput.value = config.port;
    }
    uploadSwitch.checked = config.uploadsEnabled;
    isRunning = config.isRunning;

    if (isRunning) {
        statusDot.classList.add('active');
        statusDot.classList.remove('stopped');
        statusText.innerText = "RUNNING";
        startBtn.innerText = "Stop Server";
        infoSection.style.display = 'flex';
        sessionPin.value = config.sessionPin;
        uploadPin.value = config.uploadPin;
        networkLink.innerText = config.networkAddr;
        localLink.innerText = config.localAddr;
    } else {
        statusDot.classList.remove('active');
        statusDot.classList.add('stopped');
        statusText.innerText = "STOPPED";
        startBtn.innerText = "Start Server";
        infoSection.style.display = 'none';
        sessionPin.value = config.sessionPin;
        uploadPin.value = config.uploadPin;
    }
}

window.selectFolder = async function () {
    const folder = await SelectFolder();
    if (folder) {
        folderInput.value = folder;
    }
}

window.updateUploads = async function () {
    await SetUploadEnabled(uploadSwitch.checked);
}

window.renewSessionPin = async function () {
    const newPin = await RegenerateSessionPIN();
    sessionPin.value = newPin;
}

window.renewUploadPin = async function () {
    const newPin = await RegenerateUploadPIN();
    uploadPin.value = newPin;
}

window.showQR = async function () {
    try {
        const base64Image = await GetQRCode();
        document.getElementById('qr-image').src = 'data:image/png;base64,' + base64Image;
        document.getElementById('qr-link').innerText = networkLink.innerText;
        document.getElementById('qr-modal').style.display = 'block';
    } catch (err) {
        console.error("QR Error", err);
    }
}

window.showAbout = async function () {
    const info = await GetAboutInfo();
    document.getElementById('about-version').innerText = info.version;
    document.getElementById('about-author').innerText = info.author;
    document.getElementById('about-website').innerText = info.website;
    document.getElementById('about-modal').style.display = 'block';
}

window.hideModal = function (id) {
    document.getElementById(id).style.display = 'none';
}

window.copyNetworkLink = function () {
    const link = networkLink.innerText;
    navigator.clipboard.writeText(link);
    const originalText = networkLink.innerText;
    networkLink.innerText = "Copied!";
    setTimeout(() => {
        networkLink.innerText = originalText;
    }, 1000);
}

window.OpenInBrowser = async function () {
    await OpenInBrowser();
}

window.toggleServer = async function () {
    if (isRunning) {
        await StopServer();
    } else {
        await SetPort(portInput.value);
        await SetUploadEnabled(uploadSwitch.checked);
        const result = await StartServer();
        if (result === "NO_FOLDER") {
            ShowError("Configuration Error", "Please select a folder to share first!");
            return;
        }
    }
    await updateUI();
}

// Initial load
updateUI();
setInterval(updateUI, 5000);

// Close modal if clicked outside
window.onclick = function (event) {
    if (event.target.className === 'modal') {
        event.target.style.display = 'none';
    }
}
