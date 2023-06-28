# ptr

Package 'ptr' provides utility functions to get the pointer of the built-in type.

[![Codeship](https://img.shields.io/codeship/3696d4b0-e7f9-0133-8a9a-7ab9be0b8d5f.svg?maxAge=60)](https://codeship.com/projects/147065)

## Why?

```
text := returnsString()
takesStringPtr(&text)

text = returnsString()
str := struct {
  text *string
} {
  text: &text,
}
```

↓

```
takesStringPtr(ptr.String(returnsString()))

str := struct {
  text *string
} {
  text: ptr.String(returnsString()),
}
```

It's so trivial, but that is I want.

# License

[![MIT License](http://img.shields.io/badge/license-MIT-blue.svg)](http://www.opensource.org/licenses/MIT)

This is distributed under the [MIT License](http://www.opensource.org/licenses/MIT).

