# klbslog

generic slog handler for klb using rest & fleet

Usage:

```go
    slog.SetDefault(klbslog.New(nil))
```

Or with options:

```go
    slog.SetDefault(klbslog.New(&slog.HandlerOptions{AddSource: true, Level: slog.LevelDebug}))
```
