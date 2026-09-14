package testsrv

import (
	"bytes"
	"fmt"
	"net"
	"time"

	"golang.org/x/crypto/ssh"
)

func RunSSH(host string, port int, user, password, command string, insecureIgnoreHostKey bool) (stdout, stderr string, err error) {
	if port <= 0 {
		port = 22
	}
	cb := ssh.InsecureIgnoreHostKey()
	if !insecureIgnoreHostKey {
		// 未配置严格 known_hosts 时仍拒绝静默降级:要求显式开 insecure
		return "", "", fmt.Errorf("未启用 AVATAR_SSH_INSECURE_IGNORE_HOSTKEY,拒绝连接(请在受控环境开启)")
	}
	cfg := &ssh.ClientConfig{
		User:            user,
		Auth:            []ssh.AuthMethod{ssh.Password(password)},
		HostKeyCallback: cb,
		Timeout:         10 * time.Second,
	}
	addr := net.JoinHostPort(host, fmt.Sprintf("%d", port))
	client, err := ssh.Dial("tcp", addr, cfg)
	if err != nil {
		return "", "", fmt.Errorf("ssh dial: %w", err)
	}
	defer client.Close()

	sess, err := client.NewSession()
	if err != nil {
		return "", "", err
	}
	defer sess.Close()

	var outBuf, errBuf bytes.Buffer
	sess.Stdout = &outBuf
	sess.Stderr = &errBuf
	done := make(chan error, 1)
	go func() { done <- sess.Run(command) }()
	select {
	case err = <-done:
	case <-time.After(60 * time.Second):
		_ = sess.Close()
		err = fmt.Errorf("命令超时(60s)")
	}
	return outBuf.String(), errBuf.String(), err
}
