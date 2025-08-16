// Form Validation System for FND
// Provides client-side validation for all forms

class FormValidator {
    constructor() {
        this.validators = {
            required: (value) => value.trim() !== '' || 'This field is required',
            email: (value) => {
                const emailRegex = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;
                return emailRegex.test(value) || 'Please enter a valid email address';
            },
            url: (value) => {
                if (!value) return true; // Allow empty URLs
                const urlRegex = /^https?:\/\/.+/;
                return urlRegex.test(value) || 'Please enter a valid URL starting with http:// or https://';
            },
            port: (value) => {
                if (!value) return true; // Allow empty ports
                const port = parseInt(value);
                return (port >= 1 && port <= 65535) || 'Port must be between 1 and 65535';
            },
            positiveInteger: (value) => {
                if (!value) return true; // Allow empty values
                const num = parseInt(value);
                return (num > 0 && Number.isInteger(num)) || 'Please enter a positive integer';
            },
            token: (value) => {
                if (!value) return true; // Allow empty tokens
                return value.length >= 10 || 'Token must be at least 10 characters long';
            },
            chatId: (value) => {
                if (!value) return true; // Allow empty chat IDs
                return /^-?\d+$/.test(value) || 'Chat ID must be a number';
            },
            host: (value) => {
                if (!value) return true; // Allow empty hosts
                const hostRegex = /^[a-zA-Z0-9.-]+$/;
                return hostRegex.test(value) || 'Host must contain only letters, numbers, dots, and hyphens';
            },
            template: (value) => {
                if (!value) return true; // Allow empty templates
                // Check for basic template syntax
                const templateRegex = /{{.*?}}/;
                return templateRegex.test(value) || 'Template should contain variables like {{.Variable}}';
            }
        };
        
        this.init();
    }

    init() {
        // Add validation to all forms on page load
        this.addValidationToForms();
        
        // Listen for HTMX content updates to add validation to new forms
        document.addEventListener('htmx:afterSwap', (event) => {
            this.addValidationToForms();
        });
    }

    addValidationToForms() {
        const forms = document.querySelectorAll('form');
        forms.forEach(form => {
            this.addValidationToForm(form);
        });
    }

    addValidationToForm(form) {
        // Add validation attributes to inputs based on their names and types
        const inputs = form.querySelectorAll('input, textarea, select');
        
        inputs.forEach(input => {
            this.addValidationToInput(input);
            
            // Add real-time validation
            input.addEventListener('blur', () => this.validateInput(input));
            input.addEventListener('input', () => this.clearInputError(input));
        });

        // Add form submission validation
        form.addEventListener('submit', (e) => this.validateForm(e));
    }

    addValidationToInput(input) {
        const name = input.name || '';
        const type = input.type || '';
        const placeholder = input.placeholder || '';

        // Add validation based on input name and type
        if (name.includes('host') || name.includes('server')) {
            input.setAttribute('data-validate', 'host');
        } else if (name.includes('port')) {
            input.setAttribute('data-validate', 'port');
        } else if (name.includes('token')) {
            input.setAttribute('data-validate', 'token');
        } else if (name.includes('chatid') || name.includes('chat_id')) {
            input.setAttribute('data-validate', 'chatId');
        } else if (name.includes('email')) {
            input.setAttribute('data-validate', 'email');
        } else if (name.includes('url')) {
            input.setAttribute('data-validate', 'url');
        } else if (name.includes('title') || name.includes('message')) {
            input.setAttribute('data-validate', 'template');
        } else if (input.hasAttribute('required')) {
            input.setAttribute('data-validate', 'required');
        }

        // Add validation for number inputs
        if (type === 'number') {
            input.setAttribute('data-validate', 'positiveInteger');
        }
    }

    validateInput(input) {
        const validators = input.getAttribute('data-validate');
        if (!validators) return true;

        const value = input.value;
        const validatorList = validators.split(',');

        for (const validatorName of validatorList) {
            const validator = this.validators[validatorName.trim()];
            if (validator) {
                const result = validator(value);
                if (result !== true) {
                    this.showInputError(input, result);
                    return false;
                }
            }
        }

        this.clearInputError(input);
        return true;
    }

    validateForm(event) {
        const form = event.target;
        const inputs = form.querySelectorAll('input, textarea, select');
        let isValid = true;

        inputs.forEach(input => {
            if (!this.validateInput(input)) {
                isValid = false;
            }
        });

        if (!isValid) {
            event.preventDefault();
            event.stopPropagation();
            
            // Show form-level error message
            this.showFormError(form, 'Please correct the errors above before submitting.');
        }

        return isValid;
    }

    showInputError(input, message) {
        // Remove existing error
        this.clearInputError(input);

        // Add error class to input
        input.classList.add('is-danger');

        // Create error message element
        const errorDiv = document.createElement('p');
        errorDiv.className = 'help is-danger validation-error';
        errorDiv.textContent = message;
        errorDiv.style.marginTop = '0.25rem';
        errorDiv.style.fontSize = '0.875rem';

        // Insert error message after input
        const control = input.closest('.control');
        if (control) {
            control.parentNode.insertBefore(errorDiv, control.nextSibling);
        } else {
            input.parentNode.insertBefore(errorDiv, input.nextSibling);
        }
    }

    clearInputError(input) {
        // Remove error class
        input.classList.remove('is-danger');

        // Remove error message
        const errorDiv = input.parentNode.querySelector('.validation-error');
        if (errorDiv) {
            errorDiv.remove();
        }
    }

    showFormError(form, message) {
        // Remove existing form error
        this.clearFormError(form);

        // Create form error message
        const errorDiv = document.createElement('div');
        errorDiv.className = 'notification is-danger is-light form-validation-error';
        errorDiv.innerHTML = `
            <span class="icon">
                <i class="fas fa-exclamation-triangle"></i>
            </span>
            <span>${message}</span>
        `;

        // Insert at the top of the form
        form.insertBefore(errorDiv, form.firstChild);
    }

    clearFormError(form) {
        const errorDiv = form.querySelector('.form-validation-error');
        if (errorDiv) {
            errorDiv.remove();
        }
    }

    // Public method to validate a specific input
    validateField(input) {
        return this.validateInput(input);
    }

    // Public method to validate an entire form
    validateFormElement(form) {
        const event = { target: form, preventDefault: () => {}, stopPropagation: () => {} };
        return this.validateForm(event);
    }
}

// Initialize form validation when DOM is loaded
document.addEventListener('DOMContentLoaded', () => {
    window.formValidator = new FormValidator();
});

// Export for use in other scripts
if (typeof module !== 'undefined' && module.exports) {
    module.exports = FormValidator;
}
