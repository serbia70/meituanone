package printer

import (
	"bytes"
	"fmt"
	"net"
	"os"
	"strings"
	"time"
)

type Config struct {
	Mode      string
	Device    string
	TCPAddr   string
	StoreName string
}

type Item struct {
	Name     string `json:"name"`
	Qty      int    `json:"qty"`
	Unit     int    `json:"unit"`
	Subtotal int    `json:"subtotal"`
}

type Payload struct {
	OrderNo       string    `json:"order_no"`
	CreatedAt     time.Time `json:"created_at"`
	CustomerName  string    `json:"customer_name"`
	CustomerPhone string    `json:"customer_phone"`
	OrderType     string    `json:"order_type"`
	Address       string    `json:"address"`
	Note          string    `json:"note"`
	TotalAmount   int       `json:"total_amount"`
	Items         []Item    `json:"items"`
}

type Service struct {
	cfg Config
}

func New(cfg Config) *Service {
	return &Service{cfg: cfg}
}

func (s *Service) Print(p Payload) error {
	data := s.buildESCPOS(p)

	switch strings.ToLower(s.cfg.Mode) {
	case "stdout":
		_, err := os.Stdout.Write(data)
		return err
	case "file":
		f, err := os.OpenFile(s.cfg.Device, os.O_WRONLY, 0)
		if err != nil {
			return fmt.Errorf("open printer device: %w", err)
		}
		defer f.Close()
		_, err = f.Write(data)
		if err != nil {
			return fmt.Errorf("write printer device: %w", err)
		}
		return nil
	case "tcp":
		if s.cfg.TCPAddr == "" {
			return fmt.Errorf("printer tcp addr is empty")
		}
		conn, err := net.DialTimeout("tcp", s.cfg.TCPAddr, 5*time.Second)
		if err != nil {
			return fmt.Errorf("dial tcp printer: %w", err)
		}
		defer conn.Close()
		_, err = conn.Write(data)
		if err != nil {
			return fmt.Errorf("write tcp printer: %w", err)
		}
		return nil
	default:
		return fmt.Errorf("unsupported printer mode: %s", s.cfg.Mode)
	}
}

func (s *Service) buildESCPOS(p Payload) []byte {
	var buf bytes.Buffer

	buf.Write([]byte{0x1B, 0x40})
	buf.Write([]byte{0x1B, 0x61, 0x01})
	buf.WriteString(s.cfg.StoreName + "\n")
	buf.WriteString("ORDER RECEIPT\n")
	buf.WriteString("------------------------------\n")

	buf.Write([]byte{0x1B, 0x61, 0x00})
	buf.WriteString("Order: " + p.OrderNo + "\n")
	buf.WriteString("Time : " + p.CreatedAt.Local().Format("2006-01-02 15:04:05") + "\n")
	if p.CustomerName != "" {
		buf.WriteString("Name : " + p.CustomerName + "\n")
	}
	if p.CustomerPhone != "" {
		buf.WriteString("Phone: " + p.CustomerPhone + "\n")
	}
	buf.WriteString("Type : " + p.OrderType + "\n")
	if p.Address != "" {
		buf.WriteString("Addr : " + p.Address + "\n")
	}
	if p.Note != "" {
		buf.WriteString("Note : " + p.Note + "\n")
	}

	buf.WriteString("------------------------------\n")
	for _, it := range p.Items {
		line := fmt.Sprintf("%s x%d  %d\n", it.Name, it.Qty, it.Subtotal)
		buf.WriteString(line)
	}
	buf.WriteString("------------------------------\n")
	buf.Write([]byte{0x1B, 0x45, 0x01})
	buf.WriteString(fmt.Sprintf("TOTAL: %d\n", p.TotalAmount))
	buf.Write([]byte{0x1B, 0x45, 0x00})
	buf.WriteString("\n\n")

	buf.Write([]byte{0x1D, 0x56, 0x41, 0x10})
	return buf.Bytes()
}
