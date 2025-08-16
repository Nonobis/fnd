// Client-side Sentry integration for FND
// Captures JavaScript errors, user interactions, and performance data

class SentryClient {
    constructor() {
        this.initialized = false;
        this.init();
    }

    init() {
        // Check if Sentry is available (injected by server)
        if (typeof Sentry !== 'undefined') {
            this.initialized = true;
            this.setupErrorHandling();
            this.setupPerformanceMonitoring();
            this.setupUserInteractionTracking();
            console.log('Sentry client initialized');
        } else {
            console.log('Sentry not available on client side');
        }
    }

    setupErrorHandling() {
        // Capture unhandled JavaScript errors
        window.addEventListener('error', (event) => {
            this.captureError('JavaScript Error', {
                message: event.message,
                filename: event.filename,
                lineno: event.lineno,
                colno: event.colno,
                error: event.error?.stack,
                url: window.location.href,
                userAgent: navigator.userAgent
            });
        });

        // Capture unhandled promise rejections
        window.addEventListener('unhandledrejection', (event) => {
            this.captureError('Unhandled Promise Rejection', {
                reason: event.reason,
                url: window.location.href,
                userAgent: navigator.userAgent
            });
        });

        // Capture HTMX errors
        document.addEventListener('htmx:responseError', (event) => {
            this.captureError('HTMX Response Error', {
                status: event.detail.xhr.status,
                statusText: event.detail.xhr.statusText,
                responseText: event.detail.xhr.responseText,
                url: event.detail.requestConfig.path,
                method: event.detail.requestConfig.verb
            });
        });

        // Capture HTMX request errors
        document.addEventListener('htmx:sendError', (event) => {
            this.captureError('HTMX Send Error', {
                error: event.detail.error,
                url: event.detail.requestConfig.path,
                method: event.detail.requestConfig.verb
            });
        });
    }

    setupPerformanceMonitoring() {
        // Monitor page load performance
        if ('performance' in window) {
            window.addEventListener('load', () => {
                setTimeout(() => {
                    const perfData = performance.getEntriesByType('navigation')[0];
                    if (perfData) {
                        this.capturePerformance('Page Load', {
                            loadTime: perfData.loadEventEnd - perfData.loadEventStart,
                            domContentLoaded: perfData.domContentLoadedEventEnd - perfData.domContentLoadedEventStart,
                            firstPaint: performance.getEntriesByName('first-paint')[0]?.startTime,
                            firstContentfulPaint: performance.getEntriesByName('first-contentful-paint')[0]?.startTime,
                            url: window.location.href
                        });
                    }
                }, 0);
            });
        }
    }

    setupUserInteractionTracking() {
        // Track form submission errors
        document.addEventListener('submit', (event) => {
            const form = event.target;
            const formId = form.id || form.className || 'unknown-form';
            
            // Add error tracking to form validation
            form.addEventListener('invalid', (e) => {
                this.captureError('Form Validation Error', {
                    formId: formId,
                    field: e.target.name,
                    fieldType: e.target.type,
                    validationMessage: e.target.validationMessage,
                    url: window.location.href
                });
            }, true);
        });

        // Track AJAX errors
        const originalFetch = window.fetch;
        window.fetch = (...args) => {
            return originalFetch(...args).catch(error => {
                this.captureError('Fetch Error', {
                    url: args[0],
                    options: args[1],
                    error: error.message,
                    stack: error.stack
                });
                throw error;
            });
        };
    }

    captureError(title, data) {
        if (!this.initialized) return;

        Sentry.captureMessage(title, {
            level: 'error',
            tags: {
                component: 'client',
                url: window.location.href
            },
            contexts: {
                client: data
            }
        });
    }

    capturePerformance(name, data) {
        if (!this.initialized) return;

        Sentry.addBreadcrumb({
            category: 'performance',
            message: name,
            data: data,
            level: 'info'
        });
    }

    setUser(userId, email, username) {
        if (!this.initialized) return;

        Sentry.setUser({
            id: userId,
            email: email,
            username: username
        });
    }

    setTag(key, value) {
        if (!this.initialized) return;

        Sentry.setTag(key, value);
    }

    addBreadcrumb(message, category, data) {
        if (!this.initialized) return;

        Sentry.addBreadcrumb({
            message: message,
            category: category,
            data: data,
            level: 'info'
        });
    }
}

// Initialize Sentry client when DOM is loaded
document.addEventListener('DOMContentLoaded', () => {
    window.sentryClient = new SentryClient();
});

// Export for use in other scripts
if (typeof module !== 'undefined' && module.exports) {
    module.exports = SentryClient;
}
