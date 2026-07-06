// Copyright (c) the go-ruby-pundit/pundit authors
//
// SPDX-License-Identifier: BSD-3-Clause

package pundit

import (
	"errors"
	"testing"
)

// --- PolicyFinder naming / inflection -------------------------------------

func TestPolicyAndScopeNames(t *testing.T) {
	cases := []struct {
		name       string
		subject    Subject
		wantPolicy string
		wantScope  string
	}{
		{"plain object", Object("Post"), "PostPolicy", "PostPolicy::Scope"},
		{"class", Class("Post"), "PostPolicy", "PostPolicy::Scope"},
		{"symbol", Symbol("post"), "PostPolicy", "PostPolicy::Scope"},
		{"symbol underscore", Symbol("blog_post"), "BlogPostPolicy", "BlogPostPolicy::Scope"},
		{"symbol slash", Symbol("admin/post"), "Admin::PostPolicy", "Admin::PostPolicy::Scope"},
		{"symbol leading digit", Symbol("3d_render"), "3dRenderPolicy", "3dRenderPolicy::Scope"},
		{"model_name", Subject{ModelName: "Post"}, "PostPolicy", "PostPolicy::Scope"},
		{"class model_name", Subject{ClassModelName: "Post"}, "PostPolicy", "PostPolicy::Scope"},
		{"policy_class override", Subject{PolicyClass: "CustomPolicy"}, "CustomPolicy", "CustomPolicy::Scope"},
		{"class policy_class override", Subject{ClassPolicyClass: "CustomPolicy"}, "CustomPolicy", "CustomPolicy::Scope"},
		{"namespaced array", Array(Symbol("admin"), Object("Post")), "Admin::PostPolicy", "Admin::PostPolicy::Scope"},
		{"deep namespaced array", Array(Symbol("admin"), Class("Blog"), Object("Post")), "Admin::Blog::PostPolicy", "Admin::Blog::PostPolicy::Scope"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := NewPolicyFinder(tc.subject)
			if got := f.PolicyName(); got != tc.wantPolicy {
				t.Errorf("PolicyName = %q, want %q", got, tc.wantPolicy)
			}
			if got := f.ScopeName(); got != tc.wantScope {
				t.Errorf("ScopeName = %q, want %q", got, tc.wantScope)
			}
		})
	}
}

// --- Defined seam ---------------------------------------------------------

func TestDefinedSeam(t *testing.T) {
	// nil Defined => every derived name is treated as defined.
	e := &Engine{}
	if _, ok := e.Policy(Object("Post")); !ok {
		t.Fatal("nil Defined should treat names as defined")
	}
	// Defined returning true / false.
	yes := &Engine{Defined: func(string) bool { return true }}
	if _, ok := yes.Policy(Object("Post")); !ok {
		t.Fatal("Defined=>true should be defined")
	}
	no := &Engine{Defined: func(string) bool { return false }}
	if _, ok := no.Policy(Object("Post")); ok {
		t.Fatal("Defined=>false should be undefined")
	}
}

// --- Policy / Scope resolvers --------------------------------------------

func TestPolicyBang(t *testing.T) {
	e := &Engine{}
	name, err := e.PolicyBang(Object("Post"))
	if err != nil || name != "PostPolicy" {
		t.Fatalf("PolicyBang = %q, %v", name, err)
	}

	missing := &Engine{Defined: func(string) bool { return false }}
	_, err = missing.PolicyBang(Object("Widget"))
	var nd *NotDefinedError
	if !errors.As(err, &nd) {
		t.Fatalf("want *NotDefinedError, got %v", err)
	}
	if nd.Error() != "unable to find policy `WidgetPolicy` for `Widget`" {
		t.Fatalf("policy! message = %q", nd.Error())
	}
}

func TestScopeBang(t *testing.T) {
	e := &Engine{}
	name, err := e.ScopeBang(Object("Post"))
	if err != nil || name != "PostPolicy::Scope" {
		t.Fatalf("ScopeBang = %q, %v", name, err)
	}
	if _, ok := e.Scope(Object("Post")); !ok {
		t.Fatal("Scope should be defined")
	}

	missing := &Engine{Defined: func(string) bool { return false }}
	_, err = missing.ScopeBang(Object("Widget"))
	var nd *NotDefinedError
	if !errors.As(err, &nd) {
		t.Fatalf("want *NotDefinedError, got %v", err)
	}
	if nd.Error() != "unable to find scope `WidgetPolicy::Scope` for `Widget`" {
		t.Fatalf("scope! message = %q", nd.Error())
	}
}

// --- Authorize ------------------------------------------------------------

func TestAuthorizeSuccess(t *testing.T) {
	var gotPolicy, gotQuery string
	var gotUser, gotRecord any
	e := &Engine{Dispatch: func(policyClass, query string, user, record any) (bool, error) {
		gotPolicy, gotQuery, gotUser, gotRecord = policyClass, query, user, record
		return true, nil
	}}
	rec, err := e.Authorize("alice", Object("Post"), "update?")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Name != "Post" {
		t.Fatalf("authorize should return the record, got %+v", rec)
	}
	if gotPolicy != "PostPolicy" || gotQuery != "update?" || gotUser != "alice" {
		t.Fatalf("dispatch got %q %q user=%v", gotPolicy, gotQuery, gotUser)
	}
	if gotRecord.(Subject).Name != "Post" {
		t.Fatalf("dispatch record = %+v", gotRecord)
	}
}

func TestAuthorizeNamespacedReturnsModel(t *testing.T) {
	e := &Engine{Dispatch: func(policyClass, query string, _, _ any) (bool, error) {
		if policyClass != "Admin::PostPolicy" {
			t.Fatalf("policy = %q", policyClass)
		}
		return true, nil
	}}
	rec, err := e.Authorize("alice", Array(Symbol("admin"), Object("Post")), "show?")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Name != "Post" {
		t.Fatalf("namespaced authorize should return last element, got %+v", rec)
	}
}

func TestAuthorizeNotAuthorized(t *testing.T) {
	e := &Engine{Dispatch: func(string, string, any, any) (bool, error) { return false, nil }}
	_, err := e.Authorize("alice", Object("Post"), "destroy?")
	var na *NotAuthorizedError
	if !errors.As(err, &na) {
		t.Fatalf("want *NotAuthorizedError, got %v", err)
	}
	if na.Query != "destroy?" || na.Policy != "PostPolicy" || na.Record.Name != "Post" {
		t.Fatalf("error fields = %+v", na)
	}
	if na.Error() != "not allowed to PostPolicy#destroy? this Post" {
		t.Fatalf("message = %q", na.Error())
	}
}

func TestAuthorizeNotAuthorizedClassRecord(t *testing.T) {
	e := &Engine{Dispatch: func(string, string, any, any) (bool, error) { return false, nil }}
	_, err := e.Authorize("alice", Class("Post"), "index?")
	var na *NotAuthorizedError
	if !errors.As(err, &na) {
		t.Fatalf("want *NotAuthorizedError, got %v", err)
	}
	// A Class record renders bare (no "this "), matching MRI.
	if na.Error() != "not allowed to PostPolicy#index? Post" {
		t.Fatalf("message = %q", na.Error())
	}
}

func TestAuthorizeNotDefined(t *testing.T) {
	e := &Engine{
		Defined:  func(string) bool { return false },
		Dispatch: func(string, string, any, any) (bool, error) { t.Fatal("dispatch must not run"); return false, nil },
	}
	_, err := e.Authorize("alice", Object("Ghost"), "show?")
	var nd *NotDefinedError
	if !errors.As(err, &nd) {
		t.Fatalf("want *NotDefinedError, got %v", err)
	}
}

func TestAuthorizeDispatchError(t *testing.T) {
	boom := errors.New("dispatch boom")
	e := &Engine{Dispatch: func(string, string, any, any) (bool, error) { return false, boom }}
	_, err := e.Authorize("alice", Object("Post"), "edit?")
	if !errors.Is(err, boom) {
		t.Fatalf("want dispatch error, got %v", err)
	}
}

// --- PolicyScope ----------------------------------------------------------

func TestPolicyScopeResolves(t *testing.T) {
	e := &Engine{ResolveScope: func(scopeClass string, user, scope any) (any, error) {
		if scopeClass != "PostPolicy::Scope" || user != "alice" {
			t.Fatalf("resolve got %q user=%v", scopeClass, user)
		}
		if scope.(Subject).Name != "Post" {
			t.Fatalf("resolve scope = %+v", scope)
		}
		return []string{"visible"}, nil
	}}
	got, err := e.PolicyScope("alice", Object("Post"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rows, ok := got.([]string); !ok || len(rows) != 1 || rows[0] != "visible" {
		t.Fatalf("resolved = %#v", got)
	}
}

func TestPolicyScopeNamespacedModel(t *testing.T) {
	e := &Engine{ResolveScope: func(scopeClass string, _, scope any) (any, error) {
		if scopeClass != "Admin::PostPolicy::Scope" {
			t.Fatalf("scope class = %q", scopeClass)
		}
		if scope.(Subject).Name != "Post" {
			t.Fatalf("model = %+v", scope)
		}
		return nil, nil
	}}
	if _, err := e.PolicyScope("alice", Array(Symbol("admin"), Object("Post"))); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPolicyScopeUndefinedReturnsNil(t *testing.T) {
	e := &Engine{
		Defined:      func(string) bool { return false },
		ResolveScope: func(string, any, any) (any, error) { t.Fatal("must not resolve"); return nil, nil },
	}
	got, err := e.PolicyScope("alice", Object("Ghost"))
	if err != nil || got != nil {
		t.Fatalf("undefined scope => (nil,nil), got (%v,%v)", got, err)
	}
}

func TestPolicyScopeBang(t *testing.T) {
	e := &Engine{ResolveScope: func(string, any, any) (any, error) { return "ok", nil }}
	got, err := e.PolicyScopeBang("alice", Object("Post"))
	if err != nil || got != "ok" {
		t.Fatalf("PolicyScopeBang = (%v,%v)", got, err)
	}

	missing := &Engine{Defined: func(string) bool { return false }}
	_, err = missing.PolicyScopeBang("alice", Object("Ghost"))
	var nd *NotDefinedError
	if !errors.As(err, &nd) {
		t.Fatalf("want *NotDefinedError, got %v", err)
	}
}

// --- Verify hooks ---------------------------------------------------------

func TestVerifyHooks(t *testing.T) {
	if err := VerifyAuthorized(true); err != nil {
		t.Fatalf("performed authorize => nil, got %v", err)
	}
	err := VerifyAuthorized(false)
	var anp *AuthorizationNotPerformedError
	if !errors.As(err, &anp) {
		t.Fatalf("want *AuthorizationNotPerformedError, got %v", err)
	}
	if anp.Error() == "" {
		t.Fatal("AuthorizationNotPerformedError message empty")
	}

	if err := VerifyPolicyScoped(true); err != nil {
		t.Fatalf("performed scope => nil, got %v", err)
	}
	err = VerifyPolicyScoped(false)
	var psnp *PolicyScopingNotPerformedError
	if !errors.As(err, &psnp) {
		t.Fatalf("want *PolicyScopingNotPerformedError, got %v", err)
	}
	if psnp.Error() == "" {
		t.Fatal("PolicyScopingNotPerformedError message empty")
	}
}
