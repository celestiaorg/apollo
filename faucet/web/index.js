

function requestFunds() {
    var input = document.getElementById('addressInput').value;
    fetch(`http://localhost:1095/fund/${input}`)
    .then(response => response.json())
    .then(data => console.log(data))
    .catch(error => console.error('Error:', error));
}