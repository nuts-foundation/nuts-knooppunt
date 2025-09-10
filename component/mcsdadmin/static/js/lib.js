function addOption(elementId) {
    var option = document.getElementById(elementId);
    var newOption = option.cloneNode(true);
    newOption.id = null
    option.parentElement.appendChild(newOption);
}
