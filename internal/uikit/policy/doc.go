// Package policy provides lightweight, UI-specific permission models.
//
// Each Build* function takes the current [identity.Identity] and returns a small struct of bool flags
// that templ templates use to show/hide interactive elements (buttons, links, forms).
//
// This keeps authorization checks out of markup and centralizes them in one place.
package policy
