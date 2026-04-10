package z

func Take[Opt any, T any](opts []Opt, v *T, f func(v T) Opt) []Opt {
	return TakeIf(opts, v, func(v *T) bool { return true }, f)
}

func TakeIf[Opt any, T any](opts []Opt, v *T, cond func(v *T) bool, f func(v T) Opt) []Opt {
	if v == nil {
		return opts
	}
	if !cond(v) {
		return opts
	}

	return append(opts, f(*v))
}

func Enable[Opt any](opts []Opt, v bool, f func() Opt) []Opt {
	if !v {
		return opts
	}

	return append(opts, f())
}
