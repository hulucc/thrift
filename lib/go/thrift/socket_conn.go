/*
 * Licensed to the Apache Software Foundation (ASF) under one
 * or more contributor license agreements. See the NOTICE file
 * distributed with this work for additional information
 * regarding copyright ownership. The ASF licenses this file
 * to you under the Apache License, Version 2.0 (the
 * "License"); you may not use this file except in compliance
 * with the License. You may obtain a copy of the License at
 *
 *   http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

package thrift

import (
	"bytes"
	"io"
	"net"
	"time"
)

// socketConn is a wrapped net.Conn that tries to do connectivity check.
type socketConn struct {
	net.Conn

	buf bytes.Buffer
}

var _ net.Conn = (*socketConn)(nil)

// createSocketConnFromReturn is a language sugar to help create socketConn from
// return values of functions like net.Dial, tls.Dial, net.Listener.Accept, etc.
func createSocketConnFromReturn(conn net.Conn, err error) (*socketConn, error) {
	if err != nil {
		return nil, err
	}
	return &socketConn{
		Conn: conn,
	}, nil
}

// wrapSocketConn wraps an existing net.Conn into *socketConn.
func wrapSocketConn(conn net.Conn) *socketConn {
	// In case conn is already wrapped,
	// return it as-is and avoid double wrapping.
	if sc, ok := conn.(*socketConn); ok {
		return sc
	}

	return &socketConn{
		Conn: conn,
	}
}

// isValid checks whether there's a valid connection.
//
// It's nil safe, and returns false if sc itself is nil, or if the underlying
// connection is nil.
//
// It's the same as the previous implementation of TSocket.IsOpen and
// TSSLSocket.IsOpen before we added connectivity check.
func (sc *socketConn) isValid() bool {
	return sc != nil && sc.Conn != nil
}

// IsOpen checks whether the connection is open.
//
// It's nil safe, and returns false if sc itself is nil, or if the underlying
// connection is nil.
//
// Otherwise, it tries to do a connectivity check and returns the result.
func (sc *socketConn) IsOpen() bool {
	if !sc.isValid() {
		return false
	}
	return sc.checkConn() == nil
}

// Read implements io.Reader.
//
// On Windows, it behaves the same as the underlying net.Conn.Read.
//
// On non-Windows, it treats len(p) == 0 as a connectivity check instead of
// readability check, which means instead of blocking until there's something to
// read (readability check), or always return (0, nil) (the default behavior of
// go's stdlib implementation on non-Windows), it never blocks, and will return
// an error if the connection is lost.
func (sc *socketConn) Read(p []byte) (n int, err error) {
	if len(p) == 0 {
		return 0, sc.read0()
	}

	n, err = sc.buf.Read(p)
	if err != nil && err != io.EOF {
		return
	}
	if n == len(p) {
		return n, nil
	}
	// Continue reading from the wire.
	var newRead int
	newRead, err = sc.Conn.Read(p[n:])
	n += newRead
	return
}

// CheckReadable check readability
func (sc *socketConn) CheckReadable(timeout time.Duration) error {
	sc.Conn.SetReadDeadline(time.Now().Add(timeout))
	buf := make([]byte, 1)
	n, err := sc.Read(buf)
	if n > 0 {
		sc.buf.Write(buf[:])
	}
	sc.Conn.SetReadDeadline(time.Time{})
	return err
}
