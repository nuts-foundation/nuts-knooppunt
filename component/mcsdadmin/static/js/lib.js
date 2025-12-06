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
            
            if (child.tagName === "INPUT") {
                child.value = ""
            }
        })

        // Move the ID over to the new node so that it is just for the next increment
        newOption.id = option.id
    }
    
    option.removeAttribute('id')
    option.parentElement.appendChild(newOption);
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

function handlePayloadTypeChange(selectElement) {
    if (selectElement.value === 'other') {
        // Show the modal
        showCustomPayloadModal();

        // Reset the select to previous value (will be set after modal submission)
        selectElement.value = '';
    }
}

function showCustomPayloadModal() {
    const modal = document.getElementById('customPayloadModal');
    modal.style.display = 'block';
    document.body.style.overflow = 'hidden'; // Prevent background scrolling
}

function closeCustomPayloadModal() {
    const modal = document.getElementById('customPayloadModal');
    modal.style.display = 'none';
    document.body.style.overflow = ''; // Restore scrolling

    // Clear modal fields
    document.getElementById('modal-custom-system').value = '';
    document.getElementById('modal-custom-code').value = '';
    document.getElementById('modal-custom-display').value = '';
}

function addCustomPayloadType() {
    const modalSystem = document.getElementById('modal-custom-system');
    const modalCode = document.getElementById('modal-custom-code');
    const modalDisplay = document.getElementById('modal-custom-display');

    // Validate required fields
    if (!modalSystem.value || !modalCode.value) {
        alert('System and Code are required fields');
        return;
    }

    // Get the select element
    const select = document.getElementById('payload-type');

    // Create display text (use display if provided, otherwise use code)
    const displayText = modalDisplay.value || modalCode.value;
    const fullDisplayText = `${displayText} (custom: ${modalSystem.value})`;

    // Add new option to select with a unique identifier
    const customOptionId = 'custom_' + Date.now();
    const newOption = document.createElement('option');
    newOption.value = 'other';
    newOption.textContent = fullDisplayText;
    newOption.id = customOptionId;
    newOption.selected = true;

    // Insert before the "other" option
    const otherOption = Array.from(select.options).find(opt => opt.value === 'other' && opt.text.includes('specify custom'));
    if (otherOption) {
        select.insertBefore(newOption, otherOption);
    } else {
        select.appendChild(newOption);
    }

    // Set the hidden form fields with custom values
    document.getElementById('custom-system').value = modalSystem.value;
    document.getElementById('custom-code').value = modalCode.value;
    document.getElementById('custom-display').value = modalDisplay.value;

    // Close the modal
    closeCustomPayloadModal();
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
