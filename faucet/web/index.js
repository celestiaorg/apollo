

function requestFunds() {
    disableButton()
    var input = document.getElementById('addressInput').value;
    fetch(`http://localhost:1095/fund/${input}`)
    .then(response => response.text()) // Changed to get the response as text
    .then(data => {
        console.log(data); // Log the data
        createPopup(data); // Create a popup with the response data
        enableButton();
    })
    .catch(error => {
        console.error('Error:', error);
        createPopup(`Error: ${error}`); // Create a popup with the error message
        enableButton();
    });
}

function createPopup(text) {
    var popup = document.createElement('div');
    popup.textContent = text;
    popup.className = 'popup';
    document.body.appendChild(popup);
    setTimeout(() => {
        document.body.removeChild(popup);
    }, 3000);
}

function disableButton() {
    var button = document.getElementById('request-btn');
    button.textContent = 'Requesting Funds...';
    button.disabled = true;
    button.style.borderColor = 'gray';
    button.style.cursor = 'wait';
    button.style.color = 'gray'
}

function enableButton() {
    var button = document.getElementById('request-btn');
    button.textContent = 'Request Funds';
    button.disabled = false;
    button.style.borderColor = 'white';
    button.style.cursor = 'pointer';
    button.style.color = 'white';
}

