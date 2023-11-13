# klbslog

generic slog handler for klb using rest & fleet

Usage:

```go
    slog.SetDefault(klbslog.New(nil, nil))
```

Or with options and a parent:

```go
    slog.SetDefault(klbslog.New(&slog.HandlerOptions{AddSource: true, Level: slog.LevelDebug}, slog.NewTextHandler(os.Stderr, nil)))
```
