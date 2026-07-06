// Copyright (c) the go-ruby-pundit/pundit authors
//
// SPDX-License-Identifier: BSD-3-Clause

package pundit

import "strings"

// suffix is appended to a record's class name to form its policy class name,
// mirroring Pundit::PolicyFinder::SUFFIX ("Post" -> "PostPolicy").
const suffix = "Policy"

// Subject is the reflective view of a Ruby object that PolicyFinder inspects to
// derive a policy or scope class name. It captures exactly the questions the
// gem's PolicyFinder#find and #find_class_name ask of a subject, so the naming
// logic here can be pure Go with no Ruby runtime.
//
// The host (rbgo) builds a Subject by reflecting on a real Ruby object; plain Go
// callers use the constructors Object, Class, Symbol and Array. The precedence
// among the fields matches the gem: an Array is handled first, then a
// :policy_class override (on the object, then its class), otherwise the class
// name is derived from :model_name (object, then class), a Class, a Symbol
// (camelized) or the object's class in turn.
type Subject struct {
	// Name is the subject's class name for a plain object or a Class subject
	// (e.g. "Post"), or the symbol text for a Symbol subject (e.g. "blog_post").
	Name string
	// IsClass reports that the subject is itself a Ruby Class; Name is that
	// class's own name.
	IsClass bool
	// IsSymbol reports that the subject is a Ruby Symbol; Name is its text and
	// is camelized during resolution.
	IsSymbol bool
	// Elements, when non-nil, marks the subject as a Ruby Array used for
	// namespaced lookups such as [:admin, post]; the last element is the record
	// and the earlier ones form the module context. Must be non-empty.
	Elements []Subject
	// PolicyClass, when non-empty, means the object responds to :policy_class
	// and returns this policy class name, overriding name derivation entirely.
	PolicyClass string
	// ClassPolicyClass, when non-empty, means the object's class responds to
	// :policy_class and returns this name (checked after the object itself).
	ClassPolicyClass string
	// ModelName, when non-empty, means the object responds to :model_name
	// (ActiveModel) and returns this class name.
	ModelName string
	// ClassModelName, when non-empty, means the object's class responds to
	// :model_name and returns this name (checked after the object itself).
	ClassModelName string
	// Ruby is an opaque payload the host may attach — typically the underlying
	// Ruby object — so the Dispatch and ResolveScope seams can reach it. The
	// engine never inspects it.
	Ruby any
}

// Object returns a Subject for a plain instance of the named class, e.g.
// Object("Post") resolves to the policy "PostPolicy".
func Object(className string) Subject { return Subject{Name: className} }

// Class returns a Subject for a Ruby Class itself, e.g. Class("Post") (the Post
// class, not an instance) — it too resolves to "PostPolicy".
func Class(name string) Subject { return Subject{Name: name, IsClass: true} }

// Symbol returns a Subject for a Ruby Symbol, e.g. Symbol("blog_post"); the text
// is camelized, resolving to "BlogPostPolicy".
func Symbol(text string) Subject { return Subject{Name: text, IsSymbol: true} }

// Array returns a namespaced Subject, e.g. Array(Symbol("admin"), Object("Post"))
// resolves to "Admin::PostPolicy". It must be given at least one element; the
// last is the record and the earlier ones form the module context.
func Array(elems ...Subject) Subject { return Subject{Elements: elems} }

// PolicyFinder derives the policy and scope class names for a Subject. It is a
// faithful port of Pundit::PolicyFinder — pure string/naming logic, matching the
// gem's inflection exactly.
type PolicyFinder struct {
	// Object is the subject being resolved.
	Object Subject
}

// NewPolicyFinder returns a PolicyFinder for the given subject, mirroring
// PolicyFinder.new(object).
func NewPolicyFinder(object Subject) *PolicyFinder { return &PolicyFinder{Object: object} }

// PolicyName returns the policy class name for the subject, e.g. "PostPolicy".
// It mirrors the string that Pundit::PolicyFinder#policy would constantize.
func (f *PolicyFinder) PolicyName() string { return f.find(f.Object) }

// ScopeName returns the scope class name for the subject, e.g. "PostPolicy::Scope",
// mirroring Pundit::PolicyFinder#scope ("#{policy}::Scope").
func (f *PolicyFinder) ScopeName() string { return f.find(f.Object) + "::Scope" }

// find derives a policy class name from a subject, recursing to handle namespaced
// Array subjects. It mirrors PolicyFinder#find.
func (f *PolicyFinder) find(s Subject) string {
	switch {
	case s.Elements != nil:
		last := s.Elements[len(s.Elements)-1]
		modules := s.Elements[:len(s.Elements)-1]
		context := make([]string, len(modules))
		for i, m := range modules {
			context[i] = f.findClassName(m)
		}
		return strings.Join([]string{strings.Join(context, "::"), f.find(last)}, "::")
	case s.PolicyClass != "":
		return s.PolicyClass
	case s.ClassPolicyClass != "":
		return s.ClassPolicyClass
	default:
		return f.findClassName(s) + suffix
	}
}

// findClassName derives a subject's class name, supporting ActiveModel
// (:model_name), plain classes, symbols and object instances. It mirrors
// PolicyFinder#find_class_name.
func (f *PolicyFinder) findClassName(s Subject) string {
	switch {
	case s.ModelName != "":
		return s.ModelName
	case s.ClassModelName != "":
		return s.ClassModelName
	case s.IsClass:
		return s.Name
	case s.IsSymbol:
		return camelize(s.Name)
	default:
		return s.Name
	}
}

// camelize converts a Ruby symbol's text to a class name, e.g. "blog_post" ->
// "BlogPost" and "admin/post" -> "Admin::Post". It models the subset of
// ActiveSupport's String#camelize that Pundit relies on: each "_"- or
// "/"-delimited segment is capitalized and "/" becomes "::".
func camelize(s string) string {
	var b strings.Builder
	capNext := true
	for i := 0; i < len(s); i++ {
		switch c := s[i]; c {
		case '_':
			capNext = true
		case '/':
			b.WriteString("::")
			capNext = true
		default:
			if capNext {
				b.WriteByte(upcase(c))
				capNext = false
			} else {
				b.WriteByte(c)
			}
		}
	}
	return b.String()
}

// upcase returns c uppercased if it is an ASCII lowercase letter, else c
// unchanged (leaving digits and already-uppercase bytes as they are).
func upcase(c byte) byte {
	if c >= 'a' && c <= 'z' {
		return c - ('a' - 'A')
	}
	return c
}
