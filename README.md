# sortimports

Command `sortimports` sorts import sections of Go files.

It sorts imports into three sections: standard library, external imports and module-local imports.

Usage:

```
sortimports [-n] [package...]
```

It operates on the named packages (the current package `.` by default).

The `-n` flag causes it to show which files it would have changed without
actually changing them.
