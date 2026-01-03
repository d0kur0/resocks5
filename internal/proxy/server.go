package proxy

import (
	"context"
	"fmt"
	"io"
	"net"
	"resocks5/internal/consts"
	"resocks5/internal/state"
	"sync"
	"time"
)

var bufferPool = sync.Pool{
	New: func() interface{} {
		return make([]byte, 4*1024*1024)
	},
}

type Server struct {
	listener net.Listener
	settings *state.Settings
	ctx      context.Context
	cancel   context.CancelFunc
}

func NewServer() *Server {
	ctx, cancel := context.WithCancel(context.Background())
	return &Server{
		ctx:    ctx,
		cancel: cancel,
	}
}

func (s *Server) Start(settings *state.Settings) error {
	if s.listener != nil {
		return fmt.Errorf("server already running")
	}

	addr := net.JoinHostPort(consts.DefaultLocalHostname, fmt.Sprintf("%d", consts.DefaultLocalPort))
	lc := net.ListenConfig{
		KeepAlive: 30 * time.Second,
	}
	listener, err := lc.Listen(context.Background(), "tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}

	if tcpL, ok := listener.(*net.TCPListener); ok {
		tcpL.SetDeadline(time.Time{})
	}

	s.listener = listener
	s.settings = settings

	go s.acceptLoop()

	return nil
}

func (s *Server) Stop() error {
	if s.listener == nil {
		return nil
	}

	s.cancel()
	err := s.listener.Close()
	s.listener = nil

	ctx, cancel := context.WithCancel(context.Background())
	s.ctx = ctx
	s.cancel = cancel

	return err
}

func (s *Server) IsRunning() bool {
	return s.listener != nil
}

func (s *Server) acceptLoop() {
	for {
		select {
		case <-s.ctx.Done():
			return
		default:
		}

		conn, err := s.listener.Accept()
		if err != nil {
			select {
			case <-s.ctx.Done():
				return
			default:
				continue
			}
		}

		go s.handleConnection(conn)
	}
}

func (s *Server) handleConnection(clientConn net.Conn) {
	defer clientConn.Close()

	if err := s.handleSOCKS5(clientConn); err != nil {
		return
	}
}

func (s *Server) handleSOCKS5(clientConn net.Conn) error {
	buf := make([]byte, 256)

	n, err := clientConn.Read(buf)
	if err != nil || n < 3 {
		return err
	}

	if buf[0] != 0x05 {
		return fmt.Errorf("unsupported SOCKS version")
	}

	nMethods := int(buf[1])
	if n < 2+nMethods {
		return fmt.Errorf("invalid handshake")
	}

	response := []byte{0x05, 0x00}
	if _, err := clientConn.Write(response); err != nil {
		return err
	}

	n, err = clientConn.Read(buf)
	if err != nil || n < 4 {
		return err
	}

	if buf[0] != 0x05 || buf[1] != 0x01 {
		return fmt.Errorf("unsupported command")
	}

	addrType := buf[3]
	var targetAddr string
	var targetPort int

	switch addrType {
	case 0x01:
		if n < 10 {
			return fmt.Errorf("invalid IPv4 address")
		}
		targetAddr = net.IPv4(buf[4], buf[5], buf[6], buf[7]).String()
		targetPort = int(buf[8])<<8 | int(buf[9])
	case 0x03:
		domainLen := int(buf[4])
		if n < 5+domainLen+2 {
			return fmt.Errorf("invalid domain address")
		}
		targetAddr = string(buf[5 : 5+domainLen])
		targetPort = int(buf[5+domainLen])<<8 | int(buf[5+domainLen+1])
	case 0x04:
		if n < 22 {
			return fmt.Errorf("invalid IPv6 address")
		}
		ip := make(net.IP, 16)
		copy(ip, buf[4:20])
		targetAddr = ip.String()
		targetPort = int(buf[20])<<8 | int(buf[21])
	default:
		clientConn.Write([]byte{0x05, 0x08, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00})
		return fmt.Errorf("unsupported address type")
	}

	remoteAddr := net.JoinHostPort(targetAddr, fmt.Sprintf("%d", targetPort))

	remoteConn, err := s.connectToRemote(remoteAddr)
	if err != nil {
		clientConn.Write([]byte{0x05, 0x04, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00})
		return err
	}
	defer remoteConn.Close()

	response = []byte{0x05, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	if _, err := clientConn.Write(response); err != nil {
		return err
	}

	remoteConn.SetDeadline(time.Time{})
	clientConn.SetDeadline(time.Time{})

	if tcpConn, ok := remoteConn.(*net.TCPConn); ok {
		tcpConn.SetKeepAlive(true)
		tcpConn.SetKeepAlivePeriod(30 * time.Second)
		tcpConn.SetNoDelay(true)
		tcpConn.SetReadBuffer(8 * 1024 * 1024)
		tcpConn.SetWriteBuffer(8 * 1024 * 1024)
		tcpConn.SetLinger(0)
	}
	if tcpConn, ok := clientConn.(*net.TCPConn); ok {
		tcpConn.SetKeepAlive(true)
		tcpConn.SetKeepAlivePeriod(30 * time.Second)
		tcpConn.SetNoDelay(true)
		tcpConn.SetReadBuffer(8 * 1024 * 1024)
		tcpConn.SetWriteBuffer(8 * 1024 * 1024)
		tcpConn.SetLinger(0)
	}

	errCh := make(chan error, 2)

	go func() {
		err := s.copyBuffer(remoteConn, clientConn)
		if err != nil {
			remoteConn.Close()
		}
		errCh <- err
	}()

	go func() {
		err := s.copyBuffer(clientConn, remoteConn)
		if err != nil {
			clientConn.Close()
		}
		errCh <- err
	}()

	<-errCh
	remoteConn.Close()
	clientConn.Close()
	<-errCh

	return nil
}

func (s *Server) connectToRemote(targetAddr string) (net.Conn, error) {
	remoteProxyAddr := net.JoinHostPort(s.settings.ServerAddress, fmt.Sprintf("%d", s.settings.ServerPort))

	dialer := &net.Dialer{
		Timeout:   10 * time.Second,
		KeepAlive: 30 * time.Second,
	}
	conn, err := dialer.Dial("tcp", remoteProxyAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to remote proxy: %w", err)
	}

	if tcpConn, ok := conn.(*net.TCPConn); ok {
		tcpConn.SetKeepAlive(true)
		tcpConn.SetKeepAlivePeriod(30 * time.Second)
		tcpConn.SetNoDelay(true)
		tcpConn.SetReadBuffer(8 * 1024 * 1024)
		tcpConn.SetWriteBuffer(8 * 1024 * 1024)
		tcpConn.SetLinger(0)
	}

	conn.SetDeadline(time.Now().Add(30 * time.Second))

	if err := s.authenticateRemote(conn); err != nil {
		conn.Close()
		return nil, err
	}

	if err := s.connectThroughRemote(conn, targetAddr); err != nil {
		conn.Close()
		return nil, err
	}

	return conn, nil
}

func (s *Server) authenticateRemote(conn net.Conn) error {
	authRequest := []byte{0x05, 0x01, 0x02}
	if _, err := conn.Write(authRequest); err != nil {
		return err
	}

	buf := make([]byte, 2)
	if _, err := io.ReadFull(conn, buf); err != nil {
		return err
	}

	if buf[0] != 0x05 || buf[1] != 0x02 {
		return fmt.Errorf("server does not support username/password auth")
	}

	username := []byte(s.settings.ServerLogin)
	password := []byte(s.settings.ServerPassword)

	authData := make([]byte, 3+len(username)+len(password))
	authData[0] = 0x01
	authData[1] = byte(len(username))
	copy(authData[2:], username)
	authData[2+len(username)] = byte(len(password))
	copy(authData[3+len(username):], password)

	if _, err := conn.Write(authData); err != nil {
		return err
	}

	if _, err := io.ReadFull(conn, buf); err != nil {
		return err
	}

	if buf[1] != 0x00 {
		return fmt.Errorf("authentication failed")
	}

	return nil
}

func (s *Server) connectThroughRemote(conn net.Conn, targetAddr string) error {
	host, portStr, err := net.SplitHostPort(targetAddr)
	if err != nil {
		return err
	}

	ip := net.ParseIP(host)
	var request []byte

	if ip != nil {
		if ip.To4() != nil {
			request = make([]byte, 10)
			request[0] = 0x05
			request[1] = 0x01
			request[2] = 0x00
			request[3] = 0x01
			copy(request[4:], ip.To4())
		} else {
			request = make([]byte, 22)
			request[0] = 0x05
			request[1] = 0x01
			request[2] = 0x00
			request[3] = 0x04
			copy(request[4:], ip.To16())
		}
	} else {
		hostBytes := []byte(host)
		request = make([]byte, 7+len(hostBytes))
		request[0] = 0x05
		request[1] = 0x01
		request[2] = 0x00
		request[3] = 0x03
		request[4] = byte(len(hostBytes))
		copy(request[5:], hostBytes)
	}

	var port int
	fmt.Sscanf(portStr, "%d", &port)
	request[len(request)-2] = byte(port >> 8)
	request[len(request)-1] = byte(port & 0xff)

	if _, err := conn.Write(request); err != nil {
		return err
	}

	response := make([]byte, 4)
	if _, err := io.ReadFull(conn, response); err != nil {
		return err
	}

	if response[0] != 0x05 || response[1] != 0x00 {
		return fmt.Errorf("connection failed")
	}

	addrType := response[3]
	var addrLen int
	switch addrType {
	case 0x01:
		addrLen = 4
	case 0x03:
		lenByte := make([]byte, 1)
		if _, err := io.ReadFull(conn, lenByte); err != nil {
			return err
		}
		addrLen = int(lenByte[0])
	case 0x04:
		addrLen = 16
	default:
		return fmt.Errorf("unsupported address type in response")
	}

	skipBuf := make([]byte, addrLen+2)
	if _, err := io.ReadFull(conn, skipBuf); err != nil {
		return err
	}

	return nil
}

func (s *Server) copyBuffer(dst net.Conn, src net.Conn) error {
	buf := bufferPool.Get().([]byte)
	defer bufferPool.Put(buf)

	if tcpDst, ok := dst.(*net.TCPConn); ok {
		if tcpSrc, ok := src.(*net.TCPConn); ok {
			return s.fastCopy(tcpDst, tcpSrc, buf)
		}
	}

	_, err := io.CopyBuffer(dst, src, buf)
	return err
}

func (s *Server) fastCopy(dst *net.TCPConn, src *net.TCPConn, buf []byte) error {
	for {
		nr, er := src.Read(buf)
		if nr > 0 {
			nw, ew := dst.Write(buf[:nr])
			if ew != nil {
				return ew
			}
			if nw != nr {
				return io.ErrShortWrite
			}
		}
		if er != nil {
			if er == io.EOF {
				return nil
			}
			return er
		}
	}
}
