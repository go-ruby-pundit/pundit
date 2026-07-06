// Copyright (c) the go-ruby-pundit/pundit authors
//
// SPDX-License-Identifier: BSD-3-Clause

package pundit

import (
	"os/exec"
	"strings"
	"testing"
)

// rubyBin locates a `ruby` that has the pundit gem, once. The oracle tests skip
// themselves when ruby or the gem is absent (the Windows lane and the qemu
// cross-arch lanes, where no target-arch ruby exists), so the deterministic,
// ruby-free suite alone drives the 100% coverage gate; the oracle is a
// faithfulness check that runs on developer machines and the ubuntu/macos lanes
// where the gem is installed.
func rubyBin(t *testing.T) string {
	t.Helper()
	path, err := exec.LookPath("ruby")
	if err != nil {
		t.Skip("ruby not on PATH; skipping MRI oracle")
	}
	if err := exec.Command(path, "-e", `require "pundit"`).Run(); err != nil {
		t.Skip("pundit gem not installed; skipping MRI oracle")
	}
	return path
}

// mri runs a Ruby snippet against MRI and returns its trimmed stdout.
func mri(t *testing.T, ruby, src string) string {
	t.Helper()
	out, err := exec.Command(ruby, "-e", src).CombinedOutput()
	if err != nil {
		t.Fatalf("ruby failed: %v\n%s", err, out)
	}
	return strings.TrimSpace(string(out))
}

// TestOracleInflection checks the derived policy and scope class names against
// the real Pundit::PolicyFinder for plain classes, instances, symbols (including
// underscore/slash inflection) and namespaced arrays — the cases where the gem's
// inflection is load-bearing.
func TestOracleInflection(t *testing.T) {
	ruby := rubyBin(t)

	// The Ruby side derives the policy name via PolicyFinder#find (private, so
	// invoked with send) for each subject, avoiding the need for the constants to
	// actually exist. STDOUT is one "policy|scope" pair per line.
	want := mri(t, ruby, `
require "pundit"
class Post; end
class Blog; end
def line(o)
  f = Pundit::PolicyFinder.new(o)
  name = f.send(:find, o)
  puts "#{name}|#{name}::Scope"
end
line(Post.new)
line(Post)
line(:post)
line(:blog_post)
line("admin/post".to_sym)
line("3d_render".to_sym)
line([:admin, Post])
line([:admin, Blog, Post])
`)

	subjects := []Subject{
		Object("Post"),
		Class("Post"),
		Symbol("post"),
		Symbol("blog_post"),
		Symbol("admin/post"),
		Symbol("3d_render"),
		Array(Symbol("admin"), Class("Post")),
		Array(Symbol("admin"), Class("Blog"), Class("Post")),
	}
	var got strings.Builder
	for i, s := range subjects {
		if i > 0 {
			got.WriteByte('\n')
		}
		f := NewPolicyFinder(s)
		got.WriteString(f.PolicyName() + "|" + f.ScopeName())
	}
	if got.String() != want {
		t.Fatalf("inflection mismatch:\n go:\n%s\nmri:\n%s", got.String(), want)
	}
}

// TestOracleNotAuthorizedMessage checks the NotAuthorizedError message byte for
// byte against a real Pundit.authorize that fails, for both an instance record
// and a Class record (which render differently).
func TestOracleNotAuthorizedMessage(t *testing.T) {
	ruby := rubyBin(t)

	want := mri(t, ruby, `
require "pundit"
class Post; end
class PostPolicy
  def initialize(user, record); end
  def update?; false; end
  def index?; false; end
end
[[Post.new, :update?], [Post, :index?]].each do |record, query|
  begin
    Pundit.authorize(nil, record, query)
  rescue Pundit::NotAuthorizedError => e
    puts e.message
  end
end
`)

	e := &Engine{Dispatch: func(string, string, any, any) (bool, error) { return false, nil }}
	_, err1 := e.Authorize(nil, Object("Post"), "update?")
	_, err2 := e.Authorize(nil, Class("Post"), "index?")
	got := err1.Error() + "\n" + err2.Error()
	if got != want {
		t.Fatalf("message mismatch:\n go:\n%s\nmri:\n%s", got, want)
	}
}

// TestOracleNotDefinedMessage checks the derived-name portion of the
// NotDefinedError messages against the gem's policy!/scope!. Only the portion up
// to " for `" is compared: the gem interpolates object.inspect (an unstable
// address like #<Widget:0x...>) after it, whereas this engine renders the record
// by class name, since it models the naming — not Ruby's inspect.
func TestOracleNotDefinedMessage(t *testing.T) {
	ruby := rubyBin(t)

	want := mri(t, ruby, `
require "pundit"
class Widget; end
f = Pundit::PolicyFinder.new(Widget.new)
begin; f.policy!; rescue Pundit::NotDefinedError => e; puts e.message.split(" for ").first; end
begin; f.scope!;  rescue Pundit::NotDefinedError => e; puts e.message.split(" for ").first; end
`)

	e := &Engine{Defined: func(string) bool { return false }}
	_, perr := e.PolicyBang(Object("Widget"))
	_, serr := e.ScopeBang(Object("Widget"))
	prefix := func(s string) string { return strings.SplitN(s, " for ", 2)[0] }
	got := prefix(perr.Error()) + "\n" + prefix(serr.Error())
	if got != want {
		t.Fatalf("not-defined message mismatch:\n go:\n%s\nmri:\n%s", got, want)
	}
}
