// Package dlna implements a small best-effort DLNA/UPnP renderer sender.
package dlna

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

const (
	ssdpAddr       = "239.255.255.250:1900"
	mediaRenderer  = "urn:schemas-upnp-org:device:MediaRenderer:1"
	avTransportURN = "urn:schemas-upnp-org:service:AVTransport:1"
)

// Device is a discovered DLNA MediaRenderer.
type Device struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	ControlURL string `json:"controlUrl"`
}

type description struct {
	Device struct {
		FriendlyName string `xml:"friendlyName"`
		ServiceList  struct {
			Service []struct {
				ServiceType string `xml:"serviceType"`
				ControlURL  string `xml:"controlURL"`
			} `xml:"service"`
		} `xml:"serviceList"`
	} `xml:"device"`
}

// Discover sends an SSDP M-SEARCH and resolves AVTransport control URLs.
// Renderers that do not answer, or whose description cannot be fetched, are skipped.
func Discover(ctx context.Context, timeout time.Duration) ([]Device, error) {
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	conn, err := net.ListenPacket("udp4", "0.0.0.0:0")
	if err != nil {
		return nil, fmt.Errorf("dlna listen: %w", err)
	}
	defer conn.Close()
	if dl, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(dl)
	} else {
		_ = conn.SetDeadline(time.Now().Add(timeout))
	}
	search := "M-SEARCH * HTTP/1.1\r\n" +
		"HOST: " + ssdpAddr + "\r\n" +
		"MAN: \"ssdp:discover\"\r\n" +
		"MX: 2\r\n" +
		"ST: " + mediaRenderer + "\r\n\r\n"
	dst, err := net.ResolveUDPAddr("udp4", ssdpAddr)
	if err != nil {
		return nil, fmt.Errorf("dlna address: %w", err)
	}
	if _, err := conn.WriteTo([]byte(search), dst); err != nil {
		return nil, fmt.Errorf("dlna search: %w", err)
	}

	locations := map[string]bool{}
	buf := make([]byte, 64<<10)
	for {
		n, _, err := conn.ReadFrom(buf)
		if err != nil {
			break // usually a read deadline
		}
		for _, line := range strings.Split(string(buf[:n]), "\n") {
			line = strings.TrimSpace(line)
			if len(line) >= 9 && strings.EqualFold(line[:9], "location:") {
				if loc := strings.TrimSpace(line[9:]); loc != "" {
					locations[loc] = true
				}
			}
		}
		if ctx.Err() != nil {
			break
		}
	}

	client := &http.Client{Timeout: 2500 * time.Millisecond}
	out := make([]Device, 0, len(locations))
	for loc := range locations {
		dev, err := describe(ctx, client, loc)
		if err != nil || dev.ControlURL == "" {
			continue
		}
		out = append(out, dev)
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return out, nil
}

func describe(ctx context.Context, client *http.Client, location string) (Device, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, location, nil)
	if err != nil {
		return Device{}, err
	}
	res, err := client.Do(req)
	if err != nil {
		return Device{}, err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return Device{}, fmt.Errorf("description status %d", res.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, 512<<10))
	if err != nil {
		return Device{}, err
	}
	var d description
	if err := xml.Unmarshal(body, &d); err != nil {
		return Device{}, err
	}
	var control string
	for _, svc := range d.Device.ServiceList.Service {
		if strings.HasPrefix(svc.ServiceType, avTransportURN) {
			control = svc.ControlURL
			break
		}
	}
	if control == "" {
		return Device{}, errors.New("no AVTransport service")
	}
	base, err := url.Parse(location)
	if err != nil {
		return Device{}, err
	}
	ref, err := url.Parse(control)
	if err != nil {
		return Device{}, err
	}
	controlURL := base.ResolveReference(ref).String()
	name := strings.TrimSpace(d.Device.FriendlyName)
	if name == "" {
		name = base.Hostname()
	}
	return Device{ID: controlURL, Name: name, ControlURL: controlURL}, nil
}

// Play sends SetAVTransportURI + Play for a video URL.
func Play(ctx context.Context, controlURL, mediaURL, title string) error {
	if !localControlAddress(controlURL) {
		return errors.New("DLNA renderer must be on a local/private address")
	}
	didl := buildDIDL(mediaURL, title)
	body := soapEnvelope("SetAVTransportURI", "<InstanceID>0</InstanceID>"+
		"<CurrentURI>"+xmlEscape(mediaURL)+"</CurrentURI>"+
		"<CurrentURIMetaData>"+xmlEscape(didl)+"</CurrentURIMetaData>")
	if err := soapCall(ctx, controlURL, "SetAVTransportURI", body); err != nil {
		return err
	}
	body = soapEnvelope("Play", "<InstanceID>0</InstanceID><Speed>1</Speed>")
	return soapCall(ctx, controlURL, "Play", body)
}

func soapCall(ctx context.Context, controlURL, action, body string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, controlURL, strings.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", `text/xml; charset="utf-8"`)
	req.Header.Set("SOAPACTION", `"`+avTransportURN+`#`+action+`"`)
	client := &http.Client{Timeout: 6 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	payload, _ := io.ReadAll(io.LimitReader(res.Body, 64<<10))
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("DLNA %s: status %d: %s", action, res.StatusCode, strings.TrimSpace(string(payload)))
	}
	return nil
}

func soapEnvelope(action, inner string) string {
	return `<?xml version="1.0" encoding="utf-8"?>` +
		`<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/">` +
		`<s:Body><u:` + action + ` xmlns:u="` + avTransportURN + `">` + inner + `</u:` + action + `></s:Body></s:Envelope>`
}

func buildDIDL(mediaURL, title string) string {
	return `<DIDL-Lite xmlns="urn:schemas-upnp-org:metadata-1-0/DIDL-Lite/" ` +
		`xmlns:dc="http://purl.org/dc/elements/1.1/" ` +
		`xmlns:upnp="urn:schemas-upnp-org:metadata-1-0/upnp/">` +
		`<item id="0" parentID="-1" restricted="1">` +
		`<dc:title>` + xmlEscape(title) + `</dc:title>` +
		`<upnp:class>object.item.videoItem</upnp:class>` +
		`<res protocolInfo="http-get:*:video/mp4:*">` + xmlEscape(mediaURL) + `</res>` +
		`</item></DIDL-Lite>`
}

func xmlEscape(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

func localControlAddress(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return false
	}
	ip := net.ParseIP(u.Hostname())
	if ip == nil {
		addrs, err := net.LookupIP(u.Hostname())
		if err != nil || len(addrs) == 0 {
			return false
		}
		ip = addrs[0]
	}
	return ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast()
}
