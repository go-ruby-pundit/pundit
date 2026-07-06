<p align="center"><img src="https://raw.githubusercontent.com/go-ruby-pundit/brand/main/social/go-ruby-pundit-pundit.png" alt="go-ruby-pundit/pundit" width="720"></p>

# pundit — go-ruby-pundit

[![Docs](https://img.shields.io/badge/docs-mkdocs--material-DC2626)](https://go-ruby-pundit.github.io/docs/)
[![License](https://img.shields.io/badge/license-BSD--3--Clause-blue)](LICENSE)
[![Go](https://img.shields.io/badge/go-1.26.4%2B-00ADD8)](https://go.dev/dl/)
[![Coverage](https://img.shields.io/badge/coverage-100%25-1a7f37)](#tests--coverage)

**A pure-Go (no cgo) model of the authorization ENGINE of Ruby's [Pundit](https://github.com/varvet/pundit) gem**
— faithful to the observable behaviour of Pundit 2.x on MRI 4.0.5. It owns the
parts of Pundit that are pure logic: resolving a record to its **policy** and
**scope** class names (`PolicyFinder`), the `authorize` / `policy` / `policy_scope`
**protocol** and its return contract, and the `Pundit::Error` hierarchy — **without
any Ruby runtime**. The Ruby-specific work of actually invoking a policy predicate
or running a scope is delegated through injectable **seams**.

It is the `pundit` backend for
[go-embedded-ruby](https://github.com/go-embedded-ruby/ruby), but is a
**standalone, reusable** module with no dependency on the Ruby runtime — a sibling
of [go-ruby-set](https://github.com/go-ruby-set/set) and
[go-ruby-connection-pool](https://github.com/go-ruby-connection-pool/connection-pool).

> **MRI-faithful, not Composition-Oriented.** Every naming and protocol decision
> matches what MRI 4.0.5 + `pundit` does, verified by a differential oracle that
> runs the real gem side by side with this package.

## The engine owns resolution + protocol; Ruby method bodies are the seam

Pundit's job splits cleanly. The parts that are pure Go here:

- **Policy resolution.** `PolicyFinder` maps a record to a policy class name
  (`Post` → `PostPolicy`) and a scope class name (`PostPolicy::Scope`), with the
  gem's exact inflection: namespaced arrays (`[:admin, post]` → `Admin::PostPolicy`),
  symbols (`:blog_post` → `BlogPostPolicy`), classes, `model_name` (ActiveModel)
  and `policy_class` overrides.
- **Protocol.** `authorize` resolves the policy, checks the query, and — on
  success — returns the record (Pundit's contract); on a false result it raises
  the modeled `NotAuthorizedError`. `policy_scope` resolves and runs the scope.
- **Errors.** `NotAuthorizedError`, `NotDefinedError`,
  `AuthorizationNotPerformedError` (and `PolicyScopingNotPerformedError`).

The one Ruby-specific part — running a policy's predicate method or a scope's
`resolve` — is a **seam** the host plugs in. The [go-embedded-ruby](https://github.com/go-embedded-ruby/ruby)
binding supplies Ruby `send` here, so `include Pundit::Authorization` behaves
exactly like MRI.

```go
// Call a policy predicate (e.g. "update?") on the named policy class.
type Dispatch func(policyClass, query string, user, record any) (bool, error)

// Run a scope class's resolve for user over scope.
type ResolveScope func(scopeClass string, user, scope any) (any, error)

// Model safe_constantize: does a policy/scope class of this name exist?
type Defined func(className string) bool
```

## Subjects are reflective views, not Go values

A record cannot be a bare Go value because Pundit's resolution asks Ruby-specific
questions of it (is it an `Array`? a `Symbol`? does it respond to `model_name` or
`policy_class`?). A `Subject` captures exactly those answers, so the naming logic
stays pure Go. The host builds one by reflecting on a real Ruby object; Go callers
use the constructors:

```go
pundit.Object("Post")                          // a Post instance          -> PostPolicy
pundit.Class("Post")                           // the Post class           -> PostPolicy
pundit.Symbol("blog_post")                     // :blog_post               -> BlogPostPolicy
pundit.Array(pundit.Symbol("admin"),
             pundit.Object("Post"))            // [:admin, post]           -> Admin::PostPolicy
pundit.Subject{ModelName: "Post"}              // responds to :model_name  -> PostPolicy
pundit.Subject{PolicyClass: "CustomPolicy"}    // responds to :policy_class -> CustomPolicy
```

## Install

```sh
go get github.com/go-ruby-pundit/pundit
```

## Usage

```go
package main

import (
	"fmt"

	"github.com/go-ruby-pundit/pundit"
)

func main() {
	// The host plugs Ruby method dispatch into the seam. Here a Go fake stands
	// in: alice may update? a Post but not destroy? it.
	e := &pundit.Engine{
		Dispatch: func(policyClass, query string, user, record any) (bool, error) {
			return query == "update?", nil
		},
		ResolveScope: func(scopeClass string, user, scope any) (any, error) {
			return []string{"visible post"}, nil
		},
	}

	// authorize returns the record on success (Pundit's contract) ...
	rec, err := e.Authorize("alice", pundit.Object("Post"), "update?")
	fmt.Println(rec.Name, err) // Post <nil>

	// ... and a *NotAuthorizedError on a false query.
	_, err = e.Authorize("alice", pundit.Object("Post"), "destroy?")
	fmt.Println(err) // not allowed to PostPolicy#destroy? this Post

	// policy_scope resolves PostPolicy::Scope and runs it via the seam.
	scope, _ := e.PolicyScope("alice", pundit.Object("Post"))
	fmt.Println(scope) // [visible post]

	// Namespaced lookups: [:admin, post] -> Admin::PostPolicy.
	fmt.Println(pundit.NewPolicyFinder(
		pundit.Array(pundit.Symbol("admin"), pundit.Object("Post"))).PolicyName())
}
```

## API

```go
// Resolution (pure naming — Pundit::PolicyFinder)
func NewPolicyFinder(object Subject) *PolicyFinder
func (f *PolicyFinder) PolicyName() string   // "PostPolicy"
func (f *PolicyFinder) ScopeName() string    // "PostPolicy::Scope"

// Subjects
func Object(className string) Subject
func Class(name string) Subject
func Symbol(text string) Subject
func Array(elems ...Subject) Subject

// Seams
type Dispatch     func(policyClass, query string, user, record any) (bool, error)
type ResolveScope func(scopeClass string, user, scope any) (any, error)
type Defined      func(className string) bool

// Engine (Pundit protocol)
type Engine struct { Dispatch; ResolveScope; Defined }
func (e *Engine) Policy(record Subject) (string, bool)        // policy   (nil if undefined)
func (e *Engine) PolicyBang(record Subject) (string, error)  // policy!  (NotDefinedError)
func (e *Engine) Scope(record Subject) (string, bool)        // scope
func (e *Engine) ScopeBang(record Subject) (string, error)   // scope!
func (e *Engine) Authorize(user any, record Subject, query string) (Subject, error)
func (e *Engine) PolicyScope(user any, scope Subject) (any, error)       // nil if undefined
func (e *Engine) PolicyScopeBang(user any, scope Subject) (any, error)   // NotDefinedError

// Controller-hook helpers
func VerifyAuthorized(performed bool) error   // AuthorizationNotPerformedError
func VerifyPolicyScoped(performed bool) error // PolicyScopingNotPerformedError

// Errors (the Pundit::Error subtree)
type NotAuthorizedError struct { Query string; Record Subject; Policy string; Msg string }
type NotDefinedError struct { Msg string }
type AuthorizationNotPerformedError struct { Msg string }
type PolicyScopingNotPerformedError struct { AuthorizationNotPerformedError } // is-a AuthorizationNotPerformed
```

`Authorize` returns the record (the `pundit_model` — the last element of a
namespaced `Array` subject, or the record itself), matching Pundit's contract of
always returning the passed object on success. `Policy` / `Scope` return
`("", false)` when the class is undefined (mirroring `safe_constantize` returning
`nil`); the bang variants return a `*NotDefinedError` instead.

## Fidelity basis

The naming is verified **byte-for-byte against the real gem** (pundit 2.5.2 on
MRI 4.0.5) by `oracle_test.go`: the derived policy/scope names for instances,
classes, symbols (underscore and slash inflection), and namespaced arrays are
compared to `Pundit::PolicyFinder#find`; the `NotAuthorizedError` message is
compared to a real failing `Pundit.authorize` (for both an instance and a `Class`
record, which render differently); and the `NotDefinedError` name portion is
compared to `policy!` / `scope!`. The gem interpolates `object.inspect` (an
unstable object address) into the not-defined message — this engine models the
*naming*, not Ruby's `inspect`, so it renders the record by class name and the
oracle compares only the stable, derived portion.

## Tests & coverage

The deterministic, gem-free tests alone hold coverage at **100%** (so the qemu
cross-arch and Windows lanes pass the gate), paired with the differential MRI
oracle that skips itself where the `pundit` gem is absent.

```sh
COVERPKG=$(go list ./... | paste -sd, -)
go test -race -coverpkg="$COVERPKG" -coverprofile=cover.out ./...
go tool cover -func=cover.out | tail -1   # 100.0%
```

CGO-free, dependency-free, `gofmt` + `go vet` clean, and green across the six
64-bit Go targets (amd64, arm64, riscv64, loong64, ppc64le, s390x) and three OSes
(Linux, macOS, Windows).

## License

BSD-3-Clause — see [LICENSE](LICENSE). Copyright the go-ruby-pundit/pundit authors.
