package client

import "github.com/tinywasm/fetch"

// doSync blocks until req's async Send callback fires, and returns its
// result directly — router.HandlerFunc has no "respond later" mechanism in
// this ecosystem, so a server-to-server call made from inside a handler
// must resolve before the handler returns. tinywasm/fetch is callback-only
// (it also runs inside a WASM/edge binary, where blocking I/O does not
// exist); this is the minimal adapter, not a reimplementation of fetch.
func doSync(req *fetch.Request) (*fetch.Response, error) {
	type result struct {
		resp *fetch.Response
		err  error
	}
	ch := make(chan result, 1)
	req.Send(func(resp *fetch.Response, err error) {
		ch <- result{resp, err}
	})
	r := <-ch
	return r.resp, r.err
}
