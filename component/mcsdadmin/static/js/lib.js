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

            // Remove custom options from cloned select elements
            if (child.tagName === "SELECT") {
                let customOptions = Array.from(child.options).filter(opt =>
                    opt.value === 'other' && opt.hasAttribute('data-custom-system')
                );
                customOptions.forEach(opt => opt.remove());
                // Reset to default empty selection
                child.selectedIndex = 0;
            }
        })

        // Move the ID over to the new node so that it is just for the next increment
        newOption.id = option.id
    }
    
    option.removeAttribute('id')
    option.parentElement.appendChild(newOption);
}

const indexRe = /.+\[(\d+)\]/;

function incrementIndex (name) {
    if (typeof name === "string") {
        let match = name.match(indexRe)
        if (match && match.length > 1) {
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

// Track which select element triggered the modal
let currentPayloadTypeSelect = null;

function handlePayloadTypeChange(selectElement) {
    if (selectElement.value === 'other') {
        // Store reference to the select that triggered the modal
        currentPayloadTypeSelect = selectElement;

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

    // Ensure we have a reference to the select that triggered the modal
    if (!currentPayloadTypeSelect) {
        alert('Error: No select element reference found');
        return;
    }

    const select = currentPayloadTypeSelect;

    // Extract the index from the select's name (e.g., "payload-type[0]" -> "0")
    const indexMatch = select.name.match(/\[(\d+)\]/);
    if (!indexMatch) {
        alert('Error: Could not determine select index');
        return;
    }
    const index = indexMatch[1];

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

    // Store custom values as data attributes on the option element
    newOption.setAttribute('data-custom-system', modalSystem.value);
    newOption.setAttribute('data-custom-code', modalCode.value);
    newOption.setAttribute('data-custom-display', modalDisplay.value);

    // Insert before the "other" option
    const otherOption = Array.from(select.options).find(opt => opt.value === 'other' && opt.text.includes('specify custom'));
    if (otherOption) {
        select.insertBefore(newOption, otherOption);
    } else {
        select.appendChild(newOption);
    }

    // Clear the reference
    currentPayloadTypeSelect = null;

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

    // Add form submit handler to populate hidden fields from selected custom options
    const form = document.querySelector('form[method="post"]');
    if (form) {
        form.addEventListener('submit', function(e) {
            // Find all payload-type selects
            const selects = document.querySelectorAll('select[name^="payload-type["]');

            selects.forEach(function(select) {
                // Extract index from select name (e.g., "payload-type[0]" -> "0")
                const indexMatch = select.name.match(/\[(\d+)\]/);
                if (!indexMatch) return;
                const index = indexMatch[1];

                // Get the selected option
                const selectedOption = select.options[select.selectedIndex];

                // If it's a custom option with data attributes, populate hidden fields
                if (selectedOption && selectedOption.value === 'other' &&
                    selectedOption.hasAttribute('data-custom-system')) {

                    const customSystemField = document.getElementById(`custom-system[${index}]`);
                    const customCodeField = document.getElementById(`custom-code[${index}]`);
                    const customDisplayField = document.getElementById(`custom-display[${index}]`);

                    if (customSystemField && customCodeField && customDisplayField) {
                        customSystemField.value = selectedOption.getAttribute('data-custom-system');
                        customCodeField.value = selectedOption.getAttribute('data-custom-code');
                        customDisplayField.value = selectedOption.getAttribute('data-custom-display') || '';
                    }
                }
            });
        });
    }
};
