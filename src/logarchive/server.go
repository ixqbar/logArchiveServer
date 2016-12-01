package logarchive

import (
	"fmt"
	"net"
	"reflect"
	"strings"
	"errors"
	"time"
	"bytes"
)

type HandlerFn func(r *Request) (ReplyWriter, error)
type CheckerFn func(request *Request) (reflect.Value, ReplyWriter)
type HashValue map[string][]byte

type Server struct {
	Addr    string
	methods map[string]HandlerFn
}

func NewServer(addr string, handler Handler) (*Server, error) {
	srv := &Server{
		Addr    : addr,
		methods : make(map[string]HandlerFn),
	}

	rh := reflect.TypeOf(handler)
	for i := 0; i < rh.NumMethod(); i++ {
		method := rh.Method(i)

		if handler.CheckShield(method.Name) {
			continue
		}

		Debugf("scan methods:%s", method.Name)

		handlerFn, err := srv.createHandlerFn(handler, &method)
		if err != nil {
			return nil, err
		}

		srv.methods[strings.ToLower(method.Name)] = handlerFn
	}

	return srv, nil
}

func (srv *Server) ListenAndServe() error {
	proto := "unix"

	if strings.Contains(srv.Addr, ":") {
		proto = "tcp"
	}

	l, e := net.Listen(proto, srv.Addr)
	if e != nil {
		fmt.Errorf("run failed:%s", e.Error())
		return e
	}

	return srv.Serve(l)
}

func (srv *Server) Serve(l net.Listener) error {
	defer l.Close()

	for {
		rw, err := l.Accept()
		if err != nil {
			return err
		}
		go srv.ServeClient(rw)
	}

	Debugf("server accept failed")

	return nil
}

func (srv *Server) ServeClient(conn net.Conn) (err error) {
	clientChan := make(chan struct{})

	defer func() {
		conn.Close()
		close(clientChan)
	}()

	var clientAddr string

	switch co := conn.(type) {
	case *net.UnixConn:
		f, err := conn.(*net.UnixConn).File()
		if err != nil {
			return err
		}
		clientAddr = f.Name()
	default:
		clientAddr = co.RemoteAddr().String()
	}

	for {
		request, err := parseRequest(conn)
		if err != nil {
			reply := NewErrorReply(err.Error())

			if _, err = reply.WriteTo(conn); err != nil {
				return err
			}

			return err
		}

		request.Host = clientAddr
		reply, err := srv.Apply(request)
		if err != nil {
			reply = NewErrorReply(err.Error())
		}

		if _, err = reply.WriteTo(conn); err != nil {
			return err
		}
	}

	return nil
}

func (srv *Server) Apply(r *Request) (ReplyWriter, error) {
	if srv == nil || srv.methods == nil {
		return ErrMethodNotSupported, nil
	}

	fn, exists := srv.methods[strings.ToLower(r.Name)]
	if !exists {
		return ErrMethodNotSupported, nil
	}

	return fn(r)
}

func (srv *Server) ApplyString(r *Request) (string, error) {
	reply, err := srv.Apply(r)
	if err != nil {
		return "", err
	}

	return ReplyToString(reply)
}

func (srv *Server) createHandlerFn(autoHandler interface{}, f *reflect.Method) (HandlerFn, error) {
	errorType := reflect.TypeOf(srv.createHandlerFn).Out(1)
	fType := f.Func.Type()
	checkers, err := createCheckers(autoHandler, &f.Func)
	if err != nil {
		return nil, err
	}

	if fType.NumOut() == 0 {
		return nil, errors.New("Not enough return values for method " + f.Name)
	}

	if fType.NumOut() > 2 {
		return nil, errors.New("Too many return values for method " + f.Name)
	}

	if t := fType.Out(fType.NumOut() - 1); t != errorType {
		return nil, fmt.Errorf("Last return value must be an error (not %s)", t)
	}

	return srv.handlerFn(autoHandler, &f.Func, checkers)
}

func (srv *Server) handlerFn(autoHandler interface{}, f *reflect.Value, checkers []CheckerFn) (HandlerFn, error) {
	return func(request *Request) (ReplyWriter, error) {
		input := []reflect.Value{reflect.ValueOf(autoHandler)}

		if f.Type().NumIn() - 1 != len(request.Args) {
			return ErrWrongArgsNumber,nil
		}

		for _, checker := range checkers {
			value, reply := checker(request)
			if reply != nil {
				return reply, nil
			}

			input = append(input, value)
		}

		var monitorString string
		if len(request.Args) > 0 {
			monitorString = fmt.Sprintf("%.6f [%s] \"%s\" \"%s\"",
				float64(time.Now().UTC().UnixNano())/1e9,
				request.Host,
				request.Name,
				bytes.Join(request.Args, []byte{'"', ' ', '"'}))
		} else {
			monitorString = fmt.Sprintf("%.6f [%s] \"%s\"",
				float64(time.Now().UTC().UnixNano())/1e9,
				request.Host,
				request.Name)
		}

		Debugf("%s", monitorString)

		var result []reflect.Value

		if f.Type().NumIn() == 0 {
			input = []reflect.Value{}
		} else if f.Type().In(0).AssignableTo(reflect.TypeOf(autoHandler)) == false {
			input = input[1:]
		}

		if f.Type().IsVariadic() {
			result = f.CallSlice(input)
		} else {
			result = f.Call(input)
		}

		var ret interface{}
		if ierr := result[len(result)-1].Interface(); ierr != nil {
			err := ierr.(error)
			return NewErrorReply(err.Error()), nil
		}

		if len(result) > 1 {
			ret = result[0].Interface()
			return srv.createReply(request, ret)
		}

		return &StatusReply{code: "OK"}, nil
	}, nil
}

func (srv *Server) createReply(r *Request, val interface{}) (ReplyWriter, error) {
	switch v := val.(type) {
	case []interface{}:
		return &MultiBulkReply{values: v}, nil
	case string:
		return &BulkReply{value: []byte(v)}, nil
	case [][]byte:
		if v, ok := val.([]interface{}); ok {
			return &MultiBulkReply{values: v}, nil
		}
		m := make([]interface{}, len(v), cap(v))
		for i, elem := range v {
			m[i] = elem
		}
		return &MultiBulkReply{values: m}, nil
	case []byte:
		return &BulkReply{value: v}, nil
	case HashValue:
		return hashValueReply(v)
	case map[string][]byte:
		return hashValueReply(v)
	case map[string]interface{}:
		return MultiBulkFromMap(v), nil
	case int:
		return &IntegerReply{number: v}, nil
	case *StatusReply:
		return v, nil
	default:
		return nil, fmt.Errorf("Unsupported type: %s (%T)", v, v)
	}
}
