package echo

import "net/http"

type (
	router struct {
		trees map[string]*node
		echo  *Echo
	}
	node struct {
		label   byte
		prefix  string
		has     ntype // Type of node it contains
		handler HandlerFunc
		edges   edges
		echo    *Echo
	}
	edges []*node
	ntype byte
	param struct {
		Name  string
		Value string
	}
	Params []param
)

const (
	snode ntype = iota // Static node
	pnode              // Param node
	anode              // Catch-all node
)

func NewRouter(e *Echo) (r *router) {
	r = &router{
		trees: make(map[string]*node),
		echo:  e,
	}
	for _, m := range methods {
		r.trees[m] = &node{
			prefix: "",
			edges:  edges{},
		}
	}
	return
}

func (r *router) Add(method, path string, h HandlerFunc, echo *Echo) {
	for i, l := 0, len(path); i < l; i++ {
		if path[i] == ':' {
			r.insert(method, path[:i], nil, pnode, echo)
			for ; i < l && path[i] != '/'; i++ {
			}
			if i == l {
				r.insert(method, path[:i], h, snode, echo)
				return
			}
			r.insert(method, path[:i], nil, snode, echo)
		} else if path[i] == '*' {
			r.insert(method, path[:i], h, anode, echo)
		}
	}
	r.insert(method, path, h, snode, echo)
}

func (r *router) insert(method, path string, h HandlerFunc, has ntype, echo *Echo) {
	cn := r.trees[method] // Current node as root
	search := path

	for {
		sl := len(search)
		pl := len(cn.prefix)
		l := lcp(search, cn.prefix)

		if l == 0 {
			// At root node
			cn.label = search[0]
			cn.prefix = search
			cn.has = has
			if h != nil {
				cn.handler = h
				cn.echo = echo
			}
			return
		} else if l < pl {
			// Split the node
			n := newNode(cn.prefix[l:], cn.has, cn.handler, cn.edges, cn.echo)
			cn.edges = edges{n} // Add to parent

			// Reset parent node
			cn.label = cn.prefix[0]
			cn.prefix = cn.prefix[:l]
			cn.has = snode
			cn.handler = nil
			cn.echo = nil

			if l == sl {
				// At parent node
				cn.handler = h
				cn.echo = echo
			} else {
				// Need to create a node
				n = newNode(search[l:], has, h, edges{}, echo)
				cn.edges = append(cn.edges, n)
			}
			break
		} else if l < sl {
			search = search[l:]
			e := cn.findEdge(search[0])
			if e == nil {
				n := newNode(search, has, h, edges{}, echo)
				cn.edges = append(cn.edges, n)
				break
			} else {
				cn = e
			}
		} else {
			// Node already exists
			if h != nil {
				cn.handler = h
				cn.echo = echo
			}
			break
		}
	}
}

func newNode(pfx string, has ntype, h HandlerFunc, e edges, echo *Echo) (n *node) {
	n = &node{
		label:   pfx[0],
		prefix:  pfx,
		has:     has,
		handler: h,
		edges:   e,
		echo:    echo,
	}
	return
}

func (n *node) findEdge(l byte) *node {
	for _, e := range n.edges {
		if e.label == l {
			return e
		}
	}
	return nil
}

// Length of longest common prefix
func lcp(a, b string) (i int) {
	max := len(a)
	l := len(b)
	if l < max {
		max = l
	}
	for ; i < max && a[i] == b[i]; i++ {
	}
	return
}

func (r *router) Find(method, path string) (h HandlerFunc, c *Context, echo *Echo) {
	c = r.echo.pool.Get().(*Context)
	cn := r.trees[method] // Current node as root
	search := path
	n := 0 // Param count

	for {
		if search == "" || search == cn.prefix {
			// Found
			h = cn.handler
			echo = cn.echo
			return
		}

		pl := len(cn.prefix)
		l := lcp(search, cn.prefix)

		if l == pl {
			search = search[l:]
			if cn.has == pnode {
				// Param node
				cn = cn.edges[0]
				i := 0
				l = len(search)
				for ; i < l && search[i] != '/'; i++ {
				}
				p := c.params[:n+1]
				p[n].Name = cn.prefix[1:]
				p[n].Value = search[:i]
				n++
				search = search[i:]
			} else if cn.has == anode {
				// Catch-all node
				p := c.params[:n+1]
				p[n].Name = "_name"
				p[n].Value = search
				search = "" // End search
			}

			// Search complete
			if len(search) == 0 {
				continue
			}

			// Dig more
			e := cn.findEdge(search[0])
			if e == nil {
				// Not found
				return
			}
			cn = e
			continue
		}
		return
	}
}

func (r *router) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	h, c, _ := r.Find(req.Method, req.URL.Path)
	defer r.echo.pool.Put(c)
	c.Response.ResponseWriter = rw
	if h != nil {
		h(c)
	} else {
		r.echo.notFoundHandler(c)
	}
}
