// Copyright (c) the go-ruby-pundit/pundit authors
//
// SPDX-License-Identifier: BSD-3-Clause

package pundit

// The errors in this file model the Pundit::Error subtree. In the gem every one
// is a subclass of Pundit::Error < StandardError; in Go each is a distinct type
// whose pointer implements the error interface, and the single genuine subclass
// relation the engine relies on — PolicyScopingNotPerformedError inheriting
// AuthorizationNotPerformedError — is modeled by embedding.

// NotAuthorizedError mirrors Pundit::NotAuthorizedError. Authorize returns it
// when the resolved policy's query method answered false. Like the gem it
// carries the Query that was checked, the Record that was checked, and the
// Policy class name that produced the answer, and its message reads
// "not allowed to <Policy>#<query> this <Record class>".
type NotAuthorizedError struct {
	// Query is the policy predicate that was checked (e.g. "update?").
	Query string
	// Record is the pundit_model that was checked (the Subject authorized, or
	// the last element of a namespaced Array subject).
	Record Subject
	// Policy is the name of the policy class that answered the query.
	Policy string
	// Msg is the rendered message, matching MRI's NotAuthorizedError#message.
	Msg string
}

func (e *NotAuthorizedError) Error() string { return e.Msg }

// NotDefinedError mirrors Pundit::NotDefinedError. The bang resolvers
// (PolicyBang / ScopeBang and therefore Authorize / PolicyScopeBang) return it
// when no policy or scope class of the derived name is defined.
type NotDefinedError struct{ Msg string }

func (e *NotDefinedError) Error() string { return e.Msg }

// AuthorizationNotPerformedError mirrors Pundit::AuthorizationNotPerformedError.
// VerifyAuthorized returns it when an authorization check was never performed,
// exactly as Pundit's controller hook raises it when neither authorize nor
// skip_authorization ran.
type AuthorizationNotPerformedError struct{ Msg string }

func (e *AuthorizationNotPerformedError) Error() string { return e.Msg }

// PolicyScopingNotPerformedError mirrors Pundit::PolicyScopingNotPerformedError,
// which in the gem is a subclass of AuthorizationNotPerformedError. It embeds
// that type here to model the same is-a relationship. VerifyPolicyScoped returns
// it when a scoping check was never performed.
type PolicyScopingNotPerformedError struct{ AuthorizationNotPerformedError }
