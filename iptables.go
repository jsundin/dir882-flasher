package main

import (
	"github.com/coreos/go-iptables/iptables"
	"github.com/sirupsen/logrus"
)

var iptables_rules = [][]string{
	{"filter", "INPUT", "-p", "tcp", "--tcp-flags", "ALL", "RST,ACK", "-j", "DROP"},
	{"filter", "INPUT", "-p", "tcp", "--tcp-flags", "ALL", "RST", "-j", "DROP"},
	{"filter", "OUTPUT", "-p", "tcp", "--tcp-flags", "ALL", "RST,ACK", "-j", "DROP"},
	{"filter", "OUTPUT", "-p", "tcp", "--tcp-flags", "ALL", "RST", "-j", "DROP"},
}

var iptables_active bool = false

func setup_iptables() {
	logrus.Infof("adding %d iptables rules", len(iptables_rules))
	ipt, e := iptables.New()
	if e != nil {
		logrus.Panicf("could not create iptables object: %v", e)
	}

	iptables_active = true
	for i, rule := range iptables_rules {
		logrus.Debugf("inserting iptables rule #%d: %v", i+1, rule)
		if e = ipt.Insert(rule[0], rule[1], 1, rule[2:]...); e != nil {
			logrus.Panicf("could not add iptables rule #%d: %v", i+1, e)
		}
	}
}

func teardown_iptables() {
	if !iptables_active {
		return
	}
	logrus.Infof("removing %d iptables rules", len(iptables_rules))
	ipt, e := iptables.New()
	if e != nil {
		logrus.Panicf("could not create iptables object: %v", e)
	}

	for i, rule := range iptables_rules {
		logrus.Debugf("removing iptables rule #%d: %v", i+1, rule)
		if e = ipt.Delete(rule[0], rule[1], rule[2:]...); e != nil {
			logrus.Panicf("could not add iptables rule #%d: %v", i+1, e)
		}
	}
	iptables_active = false
}
