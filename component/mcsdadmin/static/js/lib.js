function addOption(elementId) {
    let option = document.getElementById(elementId);
    let newOption = option.cloneNode(true);
    let complexOption = option.tagName === "DIV"
    
    if (complexOption) {
        // Update the names of the child nodes in case of a complex option
        let childNodes = Array.from(newOption.children)
        childNodes.forEach(child => {
            let name = child.name
            child.name = incrementIndex(name)
            
        })
    } else {
        // Just copy the option in case of a simple option
        newOption.id = null;
        newOption.required = false;
        
        option.parentElement.appendChild(newOption);
    }
}

const indexRe = /.+\[(\d+)\].+/;
function incrementIndex (name) {
    if (typeof name === "string") {
        console.log(name)
        console.log(name.match(re))
    } else {
        return name
    }
} 

function removeOption(elementId) {
    let optionDiv = document.getElementById(elementId);
    let childrenCount = optionDiv.children.length;
    if (childrenCount > 1) {
        optionDiv.removeChild(optionDiv.lastChild);
    }
}

function dismissAlert(elementId) {
    let elm = document.getElementById(elementId);
    elm.hidden = true
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
