package dlna

import (
	"encoding/xml"
	"strings"
	"testing"
)

func TestDescribeServiceSelection(t *testing.T) {
	xmlBody := `<?xml version="1.0"?><root><device><friendlyName>Living Room</friendlyName><serviceList>
		<service><serviceType>urn:schemas-upnp-org:service:ConnectionManager:1</serviceType><controlURL>/cm</controlURL></service>
		<service><serviceType>urn:schemas-upnp-org:service:AVTransport:1</serviceType><controlURL>/avt</controlURL></service>
	</serviceList></device></root>`
	var d description
	if err := xml.Unmarshal([]byte(xmlBody), &d); err != nil {
		t.Fatal(err)
	}
	if d.Device.FriendlyName != "Living Room" || len(d.Device.ServiceList.Service) != 2 {
		t.Fatalf("description = %+v", d)
	}
}

func TestSOAPAndDIDLEscaping(t *testing.T) {
	didl := buildDIDL("http://nas:6500/media?a=1&b=2", "A & B <test>")
	if !strings.Contains(didl, "a=1&amp;b=2") || !strings.Contains(didl, "A &amp; B &lt;test&gt;") {
		t.Fatalf("DIDL not escaped: %s", didl)
	}
	env := soapEnvelope("Play", "<InstanceID>0</InstanceID>")
	if !strings.Contains(env, "u:Play") || !strings.Contains(env, "AVTransport") {
		t.Fatalf("SOAP envelope: %s", env)
	}
}

func TestLocalControlAddress(t *testing.T) {
	if localControlAddress("http://192.168.0.10:1234/avt") != true {
		t.Fatal("private address rejected")
	}
	if localControlAddress("http://127.0.0.1:1234/avt") != true {
		t.Fatal("loopback rejected")
	}
	if localControlAddress("http://8.8.8.8/avt") != false {
		t.Fatal("public address accepted")
	}
}
