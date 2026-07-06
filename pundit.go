// Copyright (c) the go-ruby-pundit/pundit authors
//
// SPDX-License-Identifier: BSD-3-Clause

// Package pundit is a pure-Go (no cgo) model of the authorization ENGINE of
// Ruby's Pundit gem, faithful to the observable behaviour of Pundit 2.x on
// MRI 4.0.5. It owns the parts of Pundit that are pure logic — resolving a
// record to its policy and scope class names (PolicyFinder), the authorize /
// policy / policy_scope protocol, and the Pundit::Error hierarchy — while the
// Ruby-specific work of actually invoking a policy's predicate method or running
// a scope is delegated through injectable seams. The host (rbgo) plugs its Ruby
// method dispatch into those seams, so `include Pundit::Authorization` can behave
// exactly like MRI without this package embedding a Ruby runtime.
package pundit

import "fmt"

// Dispatch invokes a policy query method (e.g. "update?") on the policy class
// named policyClass, for the given user and record, and reports the boolean the
// method returned. It is the seam through which the engine performs the
// `policy.public_send(query)` step of Pundit#authorize: the host (rbgo)
// instantiates the Ruby policy with (user, record) and sends it the query. The
// record passed is the pundit_model Subject (see Engine.Authorize).
type Dispatch func(policyClass, query string, user, record any) (bool, error)

// ResolveScope runs the scope class named scopeClass for the given user over the
// scope subject and returns the resolved collection. It is the seam for
// `PolicyScope::Scope.new(user, scope).resolve` — the host instantiates and
// resolves the Ruby scope. The scope passed is the pundit_model Subject.
type ResolveScope func(scopeClass string, user, scope any) (any, error)

// Defined reports whether a policy or scope class of the given name is defined.
// It models Ruby's String#safe_constantize: the host answers from its constant
// table so the engine can distinguish "no such policy" (Pundit#policy returning
// nil, Pundit#policy! raising NotDefinedError) from a genuine authorization
// result. When an Engine's Defined seam is nil every derived name is treated as
// defined, which suits hosts that have already validated their policies.
type Defined func(className string) bool

// Engine is the Pundit authorization engine. Construct it with the seams the
// host supplies; the zero value with only Dispatch set is enough to authorize.
type Engine struct {
	// Dispatch performs the policy predicate call. Required by Authorize.
	Dispatch Dispatch
	// ResolveScope runs a policy scope. Required by PolicyScope / PolicyScopeBang.
	ResolveScope ResolveScope
	// Defined models safe_constantize; when nil, all derived names are defined.
	Defined Defined
}

// defined answers whether className resolves to a defined class, honouring the
// Defined seam or defaulting to true when it is absent.
func (e *Engine) defined(className string) bool {
	if e.Defined == nil {
		return true
	}
	return e.Defined(className)
}

// Policy resolves the policy class name for record, returning ("", false) when no
// such policy is defined. It mirrors Pundit#policy, which returns nil in that
// case.
func (e *Engine) Policy(record Subject) (string, bool) {
	name := NewPolicyFinder(record).PolicyName()
	return name, e.defined(name)
}

// PolicyBang resolves the policy class name for record or returns a
// *NotDefinedError, mirroring Pundit#policy!.
func (e *Engine) PolicyBang(record Subject) (string, error) {
	name, ok := e.Policy(record)
	if !ok {
		return "", &NotDefinedError{Msg: fmt.Sprintf("unable to find policy `%s` for `%s`", name, record.Name)}
	}
	return name, nil
}

// Scope resolves the scope class name for record, returning ("", false) when no
// such scope is defined, mirroring Pundit::PolicyFinder#scope.
func (e *Engine) Scope(record Subject) (string, bool) {
	name := NewPolicyFinder(record).ScopeName()
	return name, e.defined(name)
}

// ScopeBang resolves the scope class name for record or returns a
// *NotDefinedError, mirroring Pundit::PolicyFinder#scope!.
func (e *Engine) ScopeBang(record Subject) (string, error) {
	name, ok := e.Scope(record)
	if !ok {
		return "", &NotDefinedError{Msg: fmt.Sprintf("unable to find scope `%s` for `%s`", name, record.Name)}
	}
	return name, nil
}

// Authorize resolves the policy for record, dispatches query against it and,
// following Pundit#authorize, returns the record (the pundit_model) on a true
// result. It returns a *NotDefinedError when no policy is defined, a
// *NotAuthorizedError when the query answered false, or whatever error Dispatch
// itself reports. For a namespaced Array subject the returned record is its last
// element, exactly as Pundit#pundit_model unwraps it.
func (e *Engine) Authorize(user any, record Subject, query string) (Subject, error) {
	policyName, err := e.PolicyBang(record)
	if err != nil {
		return Subject{}, err
	}
	model := punditModel(record)
	ok, err := e.Dispatch(policyName, query, user, model)
	if err != nil {
		return Subject{}, err
	}
	if !ok {
		return Subject{}, &NotAuthorizedError{
			Query:  query,
			Record: model,
			Policy: policyName,
			Msg:    notAuthorizedMessage(policyName, query, model),
		}
	}
	return model, nil
}

// PolicyScope resolves the scope for scope and runs it through ResolveScope,
// returning nil when no scope is defined — mirroring Pundit#policy_scope, which
// returns nil in that case.
func (e *Engine) PolicyScope(user any, scope Subject) (any, error) {
	name, ok := e.Scope(scope)
	if !ok {
		return nil, nil
	}
	return e.ResolveScope(name, user, punditModel(scope))
}

// PolicyScopeBang resolves the scope for scope and runs it, or returns a
// *NotDefinedError when no scope is defined — mirroring Pundit#policy_scope!.
func (e *Engine) PolicyScopeBang(user any, scope Subject) (any, error) {
	name, err := e.ScopeBang(scope)
	if err != nil {
		return nil, err
	}
	return e.ResolveScope(name, user, punditModel(scope))
}

// VerifyAuthorized returns a *AuthorizationNotPerformedError when performed is
// false, modeling Pundit's verify_authorized controller hook, which raises unless
// authorize (or skip_authorization) ran during the action.
func VerifyAuthorized(performed bool) error {
	if !performed {
		return &AuthorizationNotPerformedError{Msg: "Pundit authorization was not performed"}
	}
	return nil
}

// VerifyPolicyScoped returns a *PolicyScopingNotPerformedError when performed is
// false, modeling Pundit's verify_policy_scoped controller hook, which raises
// unless policy_scope (or skip_policy_scope) ran during the action.
func VerifyPolicyScoped(performed bool) error {
	if !performed {
		return &PolicyScopingNotPerformedError{AuthorizationNotPerformedError{Msg: "Pundit policy scoping was not performed"}}
	}
	return nil
}

// punditModel returns the actual record behind a possibly-namespaced subject:
// the last element of an Array subject, or the subject itself. Mirrors
// Pundit#pundit_model.
func punditModel(s Subject) Subject {
	if s.Elements != nil {
		return s.Elements[len(s.Elements)-1]
	}
	return s
}

// notAuthorizedMessage renders the NotAuthorizedError message exactly as MRI's
// Pundit::NotAuthorizedError does: "not allowed to <Policy>#<query> <record>",
// where record is the bare class name when it is a Class and "this <class>"
// otherwise.
func notAuthorizedMessage(policyName, query string, record Subject) string {
	recordName := "this " + record.Name
	if record.IsClass {
		recordName = record.Name
	}
	return fmt.Sprintf("not allowed to %s#%s %s", policyName, query, recordName)
}
