package main

import (
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

type Host struct {
	hostname string
	workload bool
}

func getHosts(fileName string) ([]Host, error) {
	file, err := os.ReadFile(fileName)
	if err != nil {
		return nil, fmt.Errorf("Can't read file %s\n", fileName)
	}
	fileStr := string(file)
	hosts := make([]Host, 0)
	for line := range strings.SplitSeq(fileStr, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		host := strings.Split(line, " ")
		if len(host) == 1 {
			hosts = append(hosts, Host{hostname: host[0], workload: false})
			continue
		}
		if len(host) == 2 && host[1] == "workload" {
			hosts = append(hosts, Host{hostname: host[0], workload: true})
			continue
		}
		return nil, fmt.Errorf("Can't parse hostname: %s\n", line)
	}
	return hosts, nil
}

func getSSHConfig(keyPath string, passphrase string, username string) (*ssh.ClientConfig, error) {
	key, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, errors.New("Can't read private key file")
	}
	var signer ssh.Signer
	if passphrase != "" {
		signer, err = ssh.ParsePrivateKeyWithPassphrase(key, []byte(passphrase))
	} else {
		signer, err = ssh.ParsePrivateKey(key)
	}
	if err != nil {
		return nil, errors.New("Can't parse private key file")
	}
	config := &ssh.ClientConfig{
		User: username,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}
	return config, nil
}

func getSession(address string, config *ssh.ClientConfig) (*ssh.Session, error) {
	client, err := ssh.Dial("tcp", address, config)
	if err != nil {
		return nil, err
	}
	session, err := client.NewSession()
	if err != nil {
		return nil, err
	}
	return session, nil
}

func rebootMachine(session *ssh.Session) error {
	err := session.Run("sudo reboot")
	if err != nil && !errors.Is(err, &ssh.ExitMissingError{}) {
		return err
	}
	return nil
}

// func drainWorkloadNode(session *ssh.Session) error {
// 	fmt.Println("Draining workload node")
// 	err := session.Run("nomad node eligibility -self -disable")
// 	if err != nil {
// 		return err
// 	}
// 	err = session.Run("nomad node drain -self")
// 	if err != nil {
// 		return err
// 	}
// 	return nil
// }
//
// func setWorkloadNodeAsEligible(session *ssh.Session) error {
// 	fmt.Println("Setting workload node eligibility back")
// 	err := session.Run("nomad node eligibility -self -enable")
// 	if err != nil {
// 		return err
// 	}
// 	return nil
// }

func waitForHost(host string, port string) error {
	address := net.JoinHostPort(host, port)
	for {
		conn, err := net.DialTimeout("tcp", address, 2*time.Second)
		if err == nil {
			conn.Close()
			break
		}
		fmt.Printf("Host %s is not reachable yet: %v\n", host, err)
		time.Sleep(time.Second)
	}
	fmt.Printf("Host %s is back online\n", host)
	return nil
}

func processHost(host Host, port string, config *ssh.ClientConfig) error {
	address := fmt.Sprintf("%s:%s", host.hostname, port)
	fmt.Printf("\nRebooting %s\n", host.hostname)
	session, err := getSession(address, config)
	if err != nil {
		return err
	}
	// if host.workload {
	// 	err = drainWorkloadNode(session)
	// 	if err != nil {
	// 		return err
	// 	}
	// }
	err = rebootMachine(session)
	if err != nil {
		return err
	}
	err = waitForHost(host.hostname, port)
	if err != nil {
		return err
	}
	// if host.workload {
	// 	err = setWorkloadNodeAsEligible(session)
	// 	if err != nil {
	// 		return err
	// 	}
	// }
	return nil
}

func main() {
	username := flag.String("user", "root", "SSH user name")
	keyPath := flag.String("key", os.ExpandEnv("$HOME/.ssh/id_rsa"), "Path to private key file")
	passphrase := flag.String("passphrase", "", "Passphrase for private key")
	port := flag.String("port", "22", "SSH port")
	fileName := flag.String("hosts", "", "File with list of hosts to reboot")
	flag.Parse()
	if *fileName == "" {
		fmt.Println("No file name passed")
		os.Exit(2)
	}
	hosts, err := getHosts(*fileName)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	config, err := getSSHConfig(*keyPath, *passphrase, *username)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	for _, host := range hosts {
		err := processHost(host, *port, config)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		time.Sleep(5 * time.Second) // Wait extra 5 seconds between each machine
	}
}
