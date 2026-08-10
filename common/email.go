package common

import (
	"bytes"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"net/smtp"
	"net/textproto"
	"slices"
	"strings"
	"time"
)

func generateMessageID() (string, error) {
	split := strings.Split(SMTPFrom, "@")
	if len(split) < 2 {
		return "", fmt.Errorf("invalid SMTP account")
	}
	domain := strings.Split(SMTPFrom, "@")[1]
	return fmt.Sprintf("<%d.%s@%s>", time.Now().UnixNano(), GetRandomString(12), domain), nil
}

func shouldUseSMTPLoginAuth() bool {
	if SMTPForceAuthLogin {
		return true
	}
	return isOutlookServer(SMTPAccount) || slices.Contains(EmailLoginAuthServerList, SMTPServer)
}

func getSMTPAuth() smtp.Auth {
	return AutoSMTPAuth(SMTPAccount, SMTPToken)
}

func shouldAuthenticateSMTP() bool {
	return SMTPAccount != "" && SMTPToken != ""
}

func smtpTLSConfig() *tls.Config {
	return &tls.Config{
		ServerName:         SMTPServer,
		InsecureSkipVerify: SMTPInsecureSkipVerify, // #nosec G402 -- admin-controlled SMTP compatibility option.
	}
}

func newSMTPClient(addr string) (*smtp.Client, error) {
	if SMTPSSLEnabled || (SMTPPort == 465 && !SMTPStartTLSEnabled) {
		conn, err := tls.Dial("tcp", addr, smtpTLSConfig())
		if err != nil {
			return nil, err
		}
		client, err := smtp.NewClient(conn, SMTPServer)
		if err != nil {
			_ = conn.Close()
			return nil, err
		}
		return client, nil
	}

	client, err := smtp.Dial(addr)
	if err != nil {
		return nil, err
	}

	if SMTPStartTLSEnabled {
		startTLSSupported, _ := client.Extension("STARTTLS")
		if !startTLSSupported {
			_ = client.Close()
			return nil, fmt.Errorf("SMTP server does not support STARTTLS")
		}
		if err := client.StartTLS(smtpTLSConfig()); err != nil {
			_ = client.Close()
			return nil, err
		}
	}

	return client, nil
}

func SendEmail(subject string, receiver string, content string) error {
	return sendEmail(subject, receiver, "", content)
}

func SendEmailWithPlainText(subject string, receiver string, plainText string, htmlContent string) error {
	return sendEmail(subject, receiver, plainText, htmlContent)
}

func sendEmail(subject string, receiver string, plainText string, htmlContent string) error {
	if SMTPFrom == "" { // for compatibility
		SMTPFrom = SMTPAccount
	}
	id, err2 := generateMessageID()
	if err2 != nil {
		return err2
	}
	if SMTPServer == "" && SMTPAccount == "" {
		return fmt.Errorf("SMTP 服务器未配置")
	}
	recipients := strings.Split(receiver, ";")
	for i := range recipients {
		recipients[i] = strings.TrimSpace(recipients[i])
	}
	from := (&mail.Address{Name: SystemName, Address: SMTPFrom}).String()
	encodedSubject := "=?UTF-8?B?" + base64.StdEncoding.EncodeToString([]byte(subject)) + "?="

	var body bytes.Buffer
	contentType := "text/html; charset=UTF-8"
	contentTransferEncoding := "quoted-printable"
	if plainText != "" {
		writer := multipart.NewWriter(&body)
		contentType = fmt.Sprintf("multipart/alternative; boundary=%q", writer.Boundary())
		contentTransferEncoding = ""

		plainHeader := make(textproto.MIMEHeader)
		plainHeader.Set("Content-Type", "text/plain; charset=UTF-8")
		plainHeader.Set("Content-Transfer-Encoding", "quoted-printable")
		plainPart, err := writer.CreatePart(plainHeader)
		if err != nil {
			return err
		}
		plainEncoder := quotedprintable.NewWriter(plainPart)
		if _, err = plainEncoder.Write([]byte(plainText)); err != nil {
			return err
		}
		if err = plainEncoder.Close(); err != nil {
			return err
		}

		htmlHeader := make(textproto.MIMEHeader)
		htmlHeader.Set("Content-Type", "text/html; charset=UTF-8")
		htmlHeader.Set("Content-Transfer-Encoding", "quoted-printable")
		htmlPart, err := writer.CreatePart(htmlHeader)
		if err != nil {
			return err
		}
		htmlEncoder := quotedprintable.NewWriter(htmlPart)
		if _, err = htmlEncoder.Write([]byte(htmlContent)); err != nil {
			return err
		}
		if err = htmlEncoder.Close(); err != nil {
			return err
		}
		if err = writer.Close(); err != nil {
			return err
		}
	} else {
		encoder := quotedprintable.NewWriter(&body)
		if _, err := encoder.Write([]byte(htmlContent)); err != nil {
			return err
		}
		if err := encoder.Close(); err != nil {
			return err
		}
	}

	var message bytes.Buffer
	fmt.Fprintf(&message, "To: %s\r\n", strings.Join(recipients, ", "))
	fmt.Fprintf(&message, "From: %s\r\n", from)
	fmt.Fprintf(&message, "Subject: %s\r\n", encodedSubject)
	fmt.Fprintf(&message, "Date: %s\r\n", time.Now().Format(time.RFC1123Z))
	fmt.Fprintf(&message, "Message-ID: %s\r\n", id)
	fmt.Fprint(&message, "MIME-Version: 1.0\r\n")
	fmt.Fprintf(&message, "Content-Type: %s\r\n", contentType)
	if contentTransferEncoding != "" {
		fmt.Fprintf(&message, "Content-Transfer-Encoding: %s\r\n", contentTransferEncoding)
	}
	fmt.Fprint(&message, "\r\n")
	message.Write(body.Bytes())
	message.WriteString("\r\n")
	mailData := message.Bytes()

	auth := getSMTPAuth()
	addr := fmt.Sprintf("%s:%d", SMTPServer, SMTPPort)
	var err error
	client, err := newSMTPClient(addr)
	if err != nil {
		return err
	}
	defer client.Close()
	if shouldAuthenticateSMTP() {
		if err = client.Auth(auth); err != nil {
			return err
		}
	}
	if err = client.Mail(SMTPFrom); err != nil {
		return err
	}
	for _, receiver := range recipients {
		if err = client.Rcpt(receiver); err != nil {
			return err
		}
	}
	w, err := client.Data()
	if err != nil {
		return err
	}
	_, err = w.Write(mailData)
	if err != nil {
		return err
	}
	err = w.Close()
	if err != nil {
		return err
	}
	err = client.Quit()
	if err != nil {
		SysError(fmt.Sprintf("failed to send email to %s: %v", receiver, err))
	}
	return err
}
