package mkot

func take[Opt any, T any](opts []Opt, v *T, f func(v T) Opt) []Opt {
	return takeIf(opts, v, func(v *T) bool { return true }, f)
}

func takeIf[Opt any, T any](opts []Opt, v *T, cond func(v *T) bool, f func(v T) Opt) []Opt {
	if v == nil {
		return opts
	}
	if !cond(v) {
		return opts
	}

	return append(opts, f(*v))
}

func enable[Opt any](opts []Opt, v bool, f func() Opt) []Opt {
	if !v {
		return opts
	}

	return append(opts, f())
}
