package main

import (
	"errors"
	"fmt"
	"github.com/akamensky/argparse"
	"github.com/miekg/dns"
	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"net"
	"os"
	"path"
	"regexp"
	"strconv"
)

/*
## Todo
- Multiple resolvers
- Caching

## Links
- https://jameshfisher.com/2017/08/04/golang-dns-server/
- https://github.com/miekg/exdns/blob/master/q/q.go
*/

var (
	defaulPort      string = "53"
	defaultResolver        = "1.1.1.1:53"
	configFileName         = ".dnese.yaml"
)

func fileExists(filename string) bool {
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}

type config struct {
	Port     string
	Rules    []rule
	Resolver string
	Always   net.IP
}

func defaultConfig() config {
	return config{
		Port:     defaulPort,
		Resolver: defaultResolver,
	}
}

func configFromFile(file string) config {
	conf := defaultConfig()

	if !fileExists(file) {
		log.Info("Config file does not exist ", file)
		return conf
	}

	log.Info("Using config file=", file)

	reader, _ := os.Open(file)
	contents, _ := ioutil.ReadAll(reader)

	yaml.Unmarshal(contents, &conf)

	for idx, rule := range conf.Rules {
		rule.Regexp = regexp.MustCompile(rule.Pattern)
		conf.Rules[idx] = rule
	}

	if len(conf.Port) == 0 {
		conf.Port = defaulPort
	}

	if len(conf.Resolver) == 0 {
		conf.Resolver = defaultResolver
	}

	return conf
}

type rule struct {
	Pattern string `json:"pattern" yaml:"pattern"`
	Address net.IP `json:"address" yaml:"address"`
	Qtype   string `json:"qtype" yaml:"qtype"`
	Regexp  *regexp.Regexp
}

type handler struct {
	Config config
}

func (h *handler) ServeDNS(w dns.ResponseWriter, r *dns.Msg) {
	msg := dns.Msg{}
	msg.SetReply(r)

	domain := msg.Question[0].Name
	qtype := dns.Type(r.Question[0].Qtype).String()

	log.WithFields(log.Fields{
		"domain": domain,
		"qtype":  qtype,
	}).Info("Serving query")

	switch r.Question[0].Qtype {
	case dns.TypeA:
		msg.Authoritative = true

		log.WithFields(log.Fields{
			"domain": domain,
			"rtype":  "A",
		}).Info("Checking rules for reply")

		for _, rule := range h.Config.Rules {
			if rule.Regexp.MatchString(domain) {
				log.WithFields(log.Fields{
					"domain": domain,
					"rtype":  "A",
				}).Info("Found rule match")

				msg.Answer = append(msg.Answer, &dns.A{
					Hdr: dns.RR_Header{
						Name:   domain,
						Rrtype: dns.TypeA,
						Class:  dns.ClassINET,
						Ttl:    60,
					},
					A: rule.Address,
				})
				w.WriteMsg(&msg)
				return
			}
		}
	}

	c := new(dns.Client)
	reply, rtt, err := c.Exchange(r, h.Config.Resolver)
	if err != nil {
		log.Error(err)
	}

	log.WithFields(log.Fields{
		"qtype": dns.Type(r.Question[0].Qtype).String(),
		"rtt":   rtt,
	}).Info("Reply timing")

	for _, answer := range reply.Answer {
		var target string
		var rtype string
		switch answer.(type) {
		case *dns.A:
			target = answer.(*dns.A).A.String()
			rtype = "A"
		case *dns.CNAME:
			target = answer.(*dns.CNAME).Target
			rtype = "CNAME"
		case *dns.MX:
			target = answer.(*dns.MX).Mx
			rtype = "MX"
		case *dns.SOA:
			target = answer.(*dns.SOA).Ns
			rtype = "SOA"
		}

		log.WithFields(log.Fields{
			"target": target,
			"rtype":  rtype,
		}).Info("Here is an answer")
	}

	w.WriteMsg(reply)
}

func main() {
	cwd, _ := os.Getwd()
	home, err := os.UserHomeDir()
	defaultConfigFile := path.Join(home, configFileName)
	configFileInCwd := path.Join(cwd, configFileName)
	conf := defaultConfig()

	if err != nil {
		panic(err)
	}

	parser := argparse.NewParser("dnese", "An amigo to help you easily serve dns queries with your answers & see what questions are being asked")
	configFile := parser.String("c", "config", &argparse.Options{
		Required: false,
		Help:     "Load config (yaml) file to use for rules and ports. Automatitcally checks for {$HOME,$PWD}/" + configFileName,
		Validate: func(args []string) error {
			if len(args) == 0 {
				return nil
			}

			if !fileExists(args[0]) {
				return errors.New("That config file does not exist")
			}
			return nil
		},
	})

	port := parser.String("p", "port", &argparse.Options{
		Required: false,
		Help:     "Port for dns server to listen to, defaults to " + defaulPort,
		Validate: func(args []string) error {
			if len(args) == 0 {
				return nil
			}

			_, err := strconv.Atoi(args[0])
			if err != nil {
				return errors.New("Invalid number supplied for --port")
			}
			return nil
		},
	})

	resolver := parser.String("r", "resolver", &argparse.Options{
		Required: false,
		Help:     "Server to forward queries on for resolution, defaults to " + defaultResolver,
		Validate: func(args []string) error {
			if len(args) == 0 {
				return nil
			}
			addr, _, err := net.SplitHostPort(args[0])
			if err != nil {
				return err
			}

			ip := net.ParseIP(addr)

			if ip != nil {
				return nil
			}
			return errors.New("Invalid ip address for --resolver")
		},
	})

	always := parser.String("a", "always", &argparse.Options{
		Required: false,
		Help:     "Provide this answer for every query",
		Validate: func(args []string) error {
			if len(args) == 0 {
				return nil
			}
			ip := net.ParseIP(args[0])

			if ip != nil {
				return nil
			}
			return errors.New("Invalid ip address for --always")
		},
	})

	err = parser.Parse(os.Args)
	if err != nil {
		fmt.Print(parser.Usage(err))
		os.Exit(1)
	}

	if len(*configFile) > 0 {
		conf = configFromFile(*configFile)
	} else if fileExists(configFileInCwd) {
		conf = configFromFile(configFileInCwd)
	} else {
		conf = configFromFile(defaultConfigFile)

	}

	// Environment variable overrides flag
	if len(os.Getenv("DNESE_PORT")) > 0 {
		conf.Port = os.Getenv("DNESE_PORT")
	} else if len(*port) > 0 {
		conf.Port = *port
	}

	if len(*always) > 0 {
		conf.Always = net.ParseIP(*always)
	}

	if len(*resolver) > 0 {
		conf.Resolver = *resolver
	}

	if conf.Always != nil {
		conf.Rules = []rule{
			{
				Address: conf.Always,
				Regexp:  regexp.MustCompile("^[a-z0-9]"),
			},
		}
	}

	if len(conf.Rules) == 0 {
		log.Warn("No rules set ... will only monitor")
	}

	srv := &dns.Server{
		Addr: ":" + conf.Port,
		Net:  "udp",
	}

	log.WithFields(log.Fields{
		"port":     conf.Port,
		"resolver": conf.Resolver,
		"always":   conf.Always,
	}).Info("Starting to listen for dns queries")

	srv.Handler = &handler{
		Config: conf,
	}

	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("Failed to set udp listener %s\n", err.Error())
	}
}
