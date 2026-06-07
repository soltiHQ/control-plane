// Package policy provides lightweight, UI-specific permission models.
//
// Each Build* function takes the current [identity.Identity] and returns a small struct of bool flags
// that templ templates use to show/hide interactive elements (buttons, links, forms).
//
// This keeps authorization checks out of markup and centralizes them in one place.
//
// These flags are cosmetic only — they hide controls the user can't use. They are
// NOT a security boundary: every mutating action is independently enforced by the
// API (the RequirePermission middleware). A stale or wrong flag here can hide or
// show a button, never grant access.
package policy
