package presentation

import "testing"

func TestDocMarkdown(t *testing.T) {
	tests := []struct{ name, doc, want string }{
		{"plain text stays", "Saves the user.", "Saves the user."},
		{"bold", "Alias of <b>get_included_files</b>", "Alias of **get_included_files**"},
		{"strong and em", "<strong>Note</strong>: <em>never</em> <i>ever</i>", "**Note**: *never* *ever*"},
		{"code", "Returns <code>true</code> on success", "Returns `true` on success"},
		{"line break", "Interface for customized serializing.<br>\nAs of PHP 8.1.0, ...", "Interface for customized serializing.\nAs of PHP 8.1.0, ..."},
		{"self-closing line break", "One<br/>Two<br />Three", "One\nTwo\nThree"},
		{"paragraphs", "First.\n<p>Second.\n</p><p>Third.</p>", "First.\n\nSecond.\n\nThird."},
		{"list", "Modes:<ul><li>one</li><li>two</li></ul>", "Modes:\n- one\n- two"},
		{"pre", "Example:<pre>$a = 1;</pre>", "Example:\n```\n$a = 1;\n```"},
		{"link with text", "See <a href=\"https://php.net/x\">the manual</a>.", "See [the manual](https://php.net/x)."},
		{"other known tags are dropped", "<span class=\"x\">kept</span> <tt>mono</tt>", "kept mono"},
		{"generics are not tags", "Returns array<string, mixed> or list<int>", "Returns array<string, mixed> or list<int>"},
		{"entities", "a &lt; b &amp;&amp; c &gt; d &quot;q&quot;&nbsp;e &#39;s", "a < b && c > d \"q\" e 's"},
		{"inline link with text", "Read {@link https://php.net/manual the manual}.", "Read [the manual](https://php.net/manual)."},
		{"inline link without text", "Read {@link https://php.net/manual}.", "Read <https://php.net/manual>."},
		{"inline link to code", "Like {@link Foo::bar()} and {@see \\App\\User}.", "Like `Foo::bar()` and `\\App\\User`."},
		{"inheritdoc", "{@inheritDoc}", ""},
		{"blank lines collapse", "<p>One</p>\n\n\n<p>Two</p>", "One\n\nTwo"},
		{"tag names ignore case", "<B>loud</B><BR>", "**loud**"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := docMarkdown(tt.doc); got != tt.want {
				t.Errorf("docMarkdown(%q)\n got %q\nwant %q", tt.doc, got, tt.want)
			}
		})
	}
}
