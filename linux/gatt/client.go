package gatt

import (
	"encoding/binary"
	"log"
	"sync"

	"github.com/currantlabs/ble/linux/att"
	"github.com/currantlabs/ble"
)

const (
	cccNotify   = 0x0001
	cccIndicate = 0x0002
)

// NewClient ...
func NewClient(conn ble.Conn) (*Client, error) {
	p := &Client{
		subs: make(map[uint16]*sub),
		conn: conn,
	}
	p.ac = att.NewClient(conn, p)
	go p.ac.Loop()
	return p, nil
}

// Client ...
type Client struct {
	sync.RWMutex

	svcs []*ble.Service
	name string
	subs map[uint16]*sub

	ac   *att.Client
	conn ble.Conn
}

// Address ...
func (p *Client) Address() ble.Addr {
	p.RLock()
	defer p.RUnlock()
	return p.conn.RemoteAddr()
}

// Name ...
func (p *Client) Name() string {
	p.RLock()
	defer p.RUnlock()
	return p.name
}

// Services ...
func (p *Client) Services() []*ble.Service {
	p.RLock()
	defer p.RUnlock()
	return p.svcs
}

// DiscoverServices ...
func (p *Client) DiscoverServices(filter []ble.UUID) ([]*ble.Service, error) {
	p.Lock()
	defer p.Unlock()
	start := uint16(0x0001)
	for {
		length, b, err := p.ac.ReadByGroupType(start, 0xFFFF, ble.PrimaryServiceUUID)
		if err == ble.ErrAttrNotFound {
			return p.svcs, nil
		}
		if err != nil {
			return nil, err
		}
		for len(b) != 0 {
			h := binary.LittleEndian.Uint16(b[:2])
			endh := binary.LittleEndian.Uint16(b[2:4])
			u := ble.UUID(b[4:length])
			if filter == nil || ble.Contains(filter, u) {
				s := &ble.Service{
					UUID:      u,
					Handle:    h,
					EndHandle: endh,
				}
				p.svcs = append(p.svcs, s)
			}
			if endh == 0xFFFF {
				return p.svcs, nil
			}
			start = endh + 1
			b = b[length:]
		}
	}
}

// DiscoverIncludedServices ...
func (p *Client) DiscoverIncludedServices(ss []ble.UUID, s *ble.Service) ([]*ble.Service, error) {
	p.Lock()
	defer p.Unlock()
	return nil, nil
}

// DiscoverCharacteristics ...
func (p *Client) DiscoverCharacteristics(filter []ble.UUID, s *ble.Service) ([]*ble.Characteristic, error) {
	p.Lock()
	defer p.Unlock()
	start := s.Handle
	var lastChar *ble.Characteristic
	for start <= s.EndHandle {
		length, b, err := p.ac.ReadByType(start, s.EndHandle, ble.CharacteristicUUID)
		if err == ble.ErrAttrNotFound {
			break
		} else if err != nil {
			return nil, err
		}
		for len(b) != 0 {
			h := binary.LittleEndian.Uint16(b[:2])
			p := ble.Property(b[2])
			vh := binary.LittleEndian.Uint16(b[3:5])
			u := ble.UUID(b[5:length])
			c := &ble.Characteristic{
				UUID:        u,
				Property:    p,
				Handle:      h,
				ValueHandle: vh,
				EndHandle:   s.EndHandle,
			}
			if filter == nil || ble.Contains(filter, u) {
				s.Characteristics = append(s.Characteristics, c)
			}
			if lastChar != nil {
				lastChar.EndHandle = c.Handle - 1
			}
			lastChar = c
			start = vh + 1
			b = b[length:]
		}
	}
	return s.Characteristics, nil
}

// DiscoverDescriptors ...
func (p *Client) DiscoverDescriptors(filter []ble.UUID, c *ble.Characteristic) ([]*ble.Descriptor, error) {
	p.Lock()
	defer p.Unlock()
	start := c.ValueHandle + 1
	for start <= c.EndHandle {
		fmt, b, err := p.ac.FindInformation(start, c.EndHandle)
		if err == ble.ErrAttrNotFound {
			break
		} else if err != nil {
			return nil, err
		}
		length := 2 + 2
		if fmt == 0x02 {
			length = 2 + 16
		}
		for len(b) != 0 {
			h := binary.LittleEndian.Uint16(b[:2])
			u := ble.UUID(b[2:length])
			d := &ble.Descriptor{UUID: u, Handle: h}
			if filter == nil || ble.Contains(filter, u) {
				c.Descriptors = append(c.Descriptors, d)
			}
			if u.Equal(ble.ClientCharacteristicConfigUUID) {
				c.CCCD = d
			}
			start = h + 1
			b = b[length:]
		}
	}
	return c.Descriptors, nil
}

// ReadCharacteristic ...
func (p *Client) ReadCharacteristic(c *ble.Characteristic) ([]byte, error) {
	p.Lock()
	defer p.Unlock()
	return p.ac.Read(c.ValueHandle)
}

// ReadLongCharacteristic ...
func (p *Client) ReadLongCharacteristic(c *ble.Characteristic) ([]byte, error) {
	p.Lock()
	defer p.Unlock()
	return nil, nil
}

// WriteCharacteristic ...
func (p *Client) WriteCharacteristic(c *ble.Characteristic, v []byte, noRsp bool) error {
	p.Lock()
	defer p.Unlock()
	if noRsp {
		p.ac.WriteCommand(c.ValueHandle, v)
		return nil
	}
	return p.ac.Write(c.ValueHandle, v)
}

// ReadDescriptor ...
func (p *Client) ReadDescriptor(d *ble.Descriptor) ([]byte, error) {
	p.Lock()
	defer p.Unlock()
	return p.ac.Read(d.Handle)
}

// WriteDescriptor ...
func (p *Client) WriteDescriptor(d *ble.Descriptor, v []byte) error {
	p.Lock()
	defer p.Unlock()
	return p.ac.Write(d.Handle, v)
}

// ReadRSSI ...
func (p *Client) ReadRSSI() int {
	p.Lock()
	defer p.Unlock()
	// TODO:
	return 0
}

// ExchangeMTU informs the server of the client’s maximum receive MTU size and
// request the server to respond with its maximum receive MTU size. [Vol 3, Part F, 3.4.2.1]
func (p *Client) ExchangeMTU(mtu int) (int, error) {
	p.Lock()
	defer p.Unlock()
	return p.ac.ExchangeMTU(mtu)
}

// Subscribe ...
func (p *Client) Subscribe(c *ble.Characteristic, ind bool, h ble.NotificationHandler) error {
	p.Lock()
	defer p.Unlock()
	if ind {
		return p.setHandlers(c.CCCD.Handle, c.ValueHandle, cccIndicate, h)
	}
	return p.setHandlers(c.CCCD.Handle, c.ValueHandle, cccNotify, h)
}

// Unsubscribe ...
func (p *Client) Unsubscribe(c *ble.Characteristic, ind bool) error {
	p.Lock()
	defer p.Unlock()
	if ind {
		return p.setHandlers(c.CCCD.Handle, c.ValueHandle, cccIndicate, nil)
	}
	return p.setHandlers(c.CCCD.Handle, c.ValueHandle, cccNotify, nil)
}

func (p *Client) setHandlers(cccdh, vh, flag uint16, h ble.NotificationHandler) error {
	s, ok := p.subs[vh]
	if !ok {
		s = &sub{cccdh, 0x0000, nil, nil}
		p.subs[vh] = s
	}
	switch {
	case h == nil && (s.ccc&flag) == 0:
		return nil
	case h != nil && (s.ccc&flag) != 0:
		return nil
	case h == nil && (s.ccc&flag) != 0:
		s.ccc &= ^uint16(flag)
	case h != nil && (s.ccc&flag) == 0:
		s.ccc |= flag
	}

	v := make([]byte, 2)
	binary.LittleEndian.PutUint16(v, s.ccc)
	if flag == cccNotify {
		s.nHandler = h
	} else {
		s.iHandler = h
	}
	return p.ac.Write(s.cccdh, v)
}

// ClearSubscriptions ...
func (p *Client) ClearSubscriptions() error {
	p.Lock()
	defer p.Unlock()
	zero := make([]byte, 2)
	for vh, s := range p.subs {
		if err := p.ac.Write(s.cccdh, zero); err != nil {
			return err
		}
		delete(p.subs, vh)
	}
	return nil
}

// CancelConnection disconnects the connection.
func (p *Client) CancelConnection() error {
	p.Lock()
	defer p.Unlock()
	return p.conn.Close()
}

// HandleNotification ...
func (p *Client) HandleNotification(req []byte) {
	p.Lock()
	defer p.Unlock()
	vh := att.HandleValueIndication(req).AttributeHandle()
	sub, ok := p.subs[vh]
	if !ok {
		// FIXME: disconnects and propagate an error to the user.
		log.Printf("Got an unregistered notification")
		return
	}
	fn := sub.nHandler
	if req[0] == att.HandleValueIndicationCode {
		fn = sub.iHandler
	}
	if fn != nil {
		fn(req[3:])
	}
}

type sub struct {
	cccdh    uint16
	ccc      uint16
	nHandler ble.NotificationHandler
	iHandler ble.NotificationHandler
}
