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
        optionDiv.removeChild(optionDiv.lastChild);
    }
}

window.onload = function(){
    htmx.config.responseHandling = [
        // 204 - No Content by default does nothing, but is not an error
        {code:"204", swap: false},
        // 200 & 300 responses are non-errors and are swapped
        {code:"[23]..", swap: true},
        // 400 & 500 we expect the server to return an alert box
        // (Server can instruct to do something else by using HX-Retarget and friends)
        {code:"[45]..", swap: true, target: "#alerts"},
        // catch all for any other response code
        {code:"...", swap: false}
    ]
};
