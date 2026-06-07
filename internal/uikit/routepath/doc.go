// Package routepath declares all URL constants used by the control-plane UI and API.
//
// Constants are split into two groups:
//   - Page* - browser-facing paths served by the UI handler (HTML pages).
//   - Api*  - JSON/REST endpoints served by the API handler.
//
// Path-builder functions (var block) append an entity ID to a base path, keeping URL construction consistent
// and typo-free across handlers, templates, and Alpine.js fetch calls.
package routepath
