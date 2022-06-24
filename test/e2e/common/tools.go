// Copyright 2022 Authors of spidernet-io
// SPDX-License-Identifier: Apache-2.0
package common

import (
	"fmt"
	"math/rand"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

func GenerateString(lenNum int) string {
	var chars = []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k", "l", "m", "n", "o", "p", "q", "r", "s", "t", "u", "v", "w", "x", "y", "z", "A", "B", "C", "D", "E", "F", "G", "H", "I", "J", "K", "L", "M", "N", "O", "P", "Q", "R", "S", "T", "U", "V", "W", "X", "Y", "Z", "1", "2", "3", "4", "5", "6", "7", "8", "9", "0"}
	str := strings.Builder{}
	length := len(chars)
	rand.Seed(time.Now().UnixNano())
	for i := 0; i < lenNum; i++ {
		str.WriteString(chars[rand.Intn(length)])
	}
	return str.String()
}

func GenerateRandomNumber(max int) string {
	rand.Seed(time.Now().UnixNano())
	randomNumber := rand.Intn(max)
	return strconv.Itoa(randomNumber)
}

func ExecCommandRebootNode(nodeMap map[string]bool) {
	for node := range nodeMap {
		session, err := gexec.Start(exec.Command("/bin/bash", "-c", fmt.Sprintf("docker restart %s", node)), GinkgoWriter, GinkgoWriter)
		Expect(err).ShouldNot(HaveOccurred())
		session.Terminate()
		Eventually(session.Exited).Should(BeClosed())
	}
}
