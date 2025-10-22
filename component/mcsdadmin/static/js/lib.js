function addOption(elementId) {
    let option = document.getElementById(elementId);
    let newOption = option.cloneNode(true);
    
    let complexOption = option.tagName === "DIV" || option.tagName === "FIELDSET";
    
    if (complexOption) {
        // Update the names of the child nodes in case of a complex option
        let childNodes = Array.from(newOption.querySelectorAll("*"));
        childNodes.forEach(child => {
            let name = child.name;
            let incrName = incrementIndex(name);
            child.name = incrName
            child.id = incrName
        })
        
        // Move the ID over to the new node so that it is just for the next increment
        newOption.id = option.id
        option.removeAttribute('id')
        
        option.parentElement.appendChild(newOption);
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
        let match = name.match(indexRe)
        if (match.length > 1) {
            let target = `[${match[1]}]`
            let replacementInt = parseInt(match[1]) +1
            let replacement = `[${replacementInt}]`
            return name.replace(target, replacement)
        } else {
            console.warn(`unknow name format for complex option: ${name}`)
            return name;
        }
    } else {
        return name;
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
    elm.hidden = true;
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
