function addOption(elementId) {
    var option = document.getElementById(elementId);
    var newOption = option.cloneNode(true);
    newOption.id = null;
    newOption.required = false;
    option.parentElement.appendChild(newOption);
}

function removeOption(elementId) {
    var optionDiv = document.getElementById(elementId);
    var childrenCount = optionDiv.children.length;
    if (childrenCount > 1) {
        optionDiv.removeChild(optionDiv.lastChild)
    }
}