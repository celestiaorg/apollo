

load()

function load() {
    fetch('/status')
    .then(response => response.json())
    .then(data => renderStatusData(data))
    .catch(error => {
        console.error('Error fetching status:', error);
        createPopup(`Error fetching status: ${error}`);
    });
}

function renderStatusData(data) {
    console.log(data)
    
    const controlPanel = document.getElementById('control-panel');
    controlPanel.innerHTML = '';
    for (const [serviceName, info] of Object.entries(data)) {
        const cardDiv = document.createElement('div');
        cardDiv.className = 'card';
        // Modify card border based on info.IsRunning
        cardDiv.style.border = info.running ? '1px solid rgb(50, 42, 152)' : '1px solid rgb(152, 48, 48)';
        cardDiv.style.backgroundColor = info.running? 'transparent' : '#2b2020'

        // Create a div for control buttons
        const controlButtonsDiv = document.createElement('div');
        controlButtonsDiv.className = 'control-buttons';

        if (!info.running) {
            const startButton = document.createElement('button');
            startButton.textContent = 'Start';
            startButton.className = 'start-button';
            startButton.onclick = () => {
                startButton.style.color = 'white';
                startButton.style.borderColor = 'white'
                startButton.textContent = 'Starting...'
                startService(serviceName);
            }
            controlButtonsDiv.appendChild(startButton);
        } else {
            const stopButton = document.createElement('button');
            stopButton.textContent = 'Stop';
            stopButton.className = 'stop-button';
            stopButton.onclick = () => {
                stopButton.style.color = 'white';
                stopButton.style.borderColor = 'white'
                stopButton.textContent = 'Stopping...'
                stopService(serviceName);
            }
            controlButtonsDiv.appendChild(stopButton);
        }

        // Append control buttons div to card div
        cardDiv.appendChild(controlButtonsDiv);

        const serviceNameDiv = document.createElement('div');
        serviceNameDiv.className = 'title';
        serviceNameDiv.textContent = convertKebabCase(serviceName);
        cardDiv.appendChild(serviceNameDiv);
        if (info.running) {
            const endpointsTitleDiv = document.createElement('div');
            endpointsTitleDiv.className = 'subtitle';
            endpointsTitleDiv.textContent = 'Endpoints:';
            cardDiv.appendChild(endpointsTitleDiv);
        }
        const endpointsDiv = document.createElement('div');
        endpointsDiv.innerHTML = Object.entries(info.provides_endpoints).map(([name, endpoint]) => 
            `<button onclick="copyEndpointToClipboard('${endpoint}')">${name}</button>`).join(' ');
        cardDiv.appendChild(endpointsDiv);

        controlPanel.appendChild(cardDiv);
    }
}

function copyEndpointToClipboard(endpoint) {
    navigator.clipboard.writeText(sanitizeEndpoint(endpoint)).then(() => {
        if (popup != null) {
            document.body.removeChild(popup);
        }
        popup = document.createElement('div');
        popup.textContent = `${endpoint} copied to clipboard!`;
        popup.className = 'popup';
        document.body.appendChild(popup);
        setTimeout(() => {
            document.body.removeChild(popup);
            popup = null;
        }, 3000);
    }).catch(err => console.error('Could not copy text: ', err));
}

function sanitizeEndpoint(endpoint) {
    var newEndpoint = endpoint.replace('tcp://', '').replace('0.0.0.0', 'localhost').replace('127.0.0.1', 'localhost');
    if (!newEndpoint.startsWith('http://')) {
        newEndpoint = 'http://' + newEndpoint;
    }
    return newEndpoint
}

function convertKebabCase(str) {
    return str.split('-').map(word => word.charAt(0).toUpperCase() + word.slice(1)).join(' ')
}

var popup

function startService(name) {
    fetch(`/start/${name}`)
    .then(response => {
        if (response.status != 200) {
            response.text().then(body => {
                createPopup(`Error starting service ${name}: ${body}`);
            });
        } else {
            console.log('Sucessfully started ' + name)
        }
        load()
    })
    .catch(error => {
        console.error('Error starting service:', error);
        createPopup(`Error starting service ${name}: ${error}`);
    });
}

function stopService(name) {
    fetch(`/stop/${name}`)
    .then(response => {
        if (response.status != 200) {
            response.text().then(body => {
                createPopup(`Error stopping service ${name}: ${body}`);
            });
        } else {
            console.log('Sucessfully stopped ' + name)
        }
        load()
    })
    .catch(error => {
        console.error('Error stopping service:', error);
        createPopup(`Error stopping service ${name}: ${error}`);
    });
}

function createPopup(text) {
    if (popup != null) {
        document.body.removeChild(popup);
    }
    popup = document.createElement('div');
    popup.textContent = text;
    popup.className = 'popup';
    document.body.appendChild(popup);
}

