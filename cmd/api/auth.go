package main

import (
 "crypto/hmac"
 "crypto/sha256"
 "database/sql"
 "encoding/base64"
 "errors"
 "net/http"
 "os"
 "strconv"
 "strings"
 "time"

 "golang.org/x/crypto/bcrypt"
)

type loginInput struct { Username string `json:"username"`; Password string `json:"password"` }

func ensureAdmin(db *sql.DB) error {
 if _,err:=db.Exec(`CREATE TABLE IF NOT EXISTS admin_users (
  id BIGSERIAL PRIMARY KEY,
  username TEXT NOT NULL UNIQUE,
  password_hash TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
 )`); err!=nil { return err }
 var count int
 if err:=db.QueryRow("SELECT COUNT(*) FROM admin_users").Scan(&count); err!=nil { return err }
 if count>0 { return nil }
 username:=strings.TrimSpace(os.Getenv("ADMIN_USERNAME"))
 password:=os.Getenv("ADMIN_PASSWORD")
 if username=="" || password=="" { return errors.New("ADMIN_USERNAME and ADMIN_PASSWORD are required for initial admin setup") }
 hash,err:=bcrypt.GenerateFromPassword([]byte(password),bcrypt.DefaultCost); if err!=nil{return err}
 _,err=db.Exec("INSERT INTO admin_users(username,password_hash) VALUES($1,$2)",username,string(hash))
 return err
}

func (s *server) login(w http.ResponseWriter,r *http.Request) {
 var in loginInput
 if decodeJSON(r,&in)!=nil { writeError(w,400,"invalid login request"); return }
 var hash string
 err:=s.db.QueryRowContext(r.Context(),"SELECT password_hash FROM admin_users WHERE username=$1",strings.TrimSpace(in.Username)).Scan(&hash)
 if err!=nil || bcrypt.CompareHashAndPassword([]byte(hash),[]byte(in.Password))!=nil {
  writeError(w,401,"username atau password salah"); return
 }
 expiry:=time.Now().Add(12*time.Hour).Unix()
 payload:=base64.RawURLEncoding.EncodeToString([]byte(strings.TrimSpace(in.Username)+"|"+strconv.FormatInt(expiry,10)))
 mac:=hmac.New(sha256.New,s.authSecret); mac.Write([]byte(payload))
 token:=payload+"."+base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
 writeJSON(w,200,map[string]any{"token":token,"username":strings.TrimSpace(in.Username),"expiresAt":expiry})
}

func (s *server) requireAdmin(next http.Handler) http.Handler {
 return http.HandlerFunc(func(w http.ResponseWriter,r *http.Request) {
  parts:=strings.SplitN(strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"),"Bearer ")), ".",2)
  if len(parts)!=2 { writeError(w,401,"admin authentication required"); return }
  mac:=hmac.New(sha256.New,s.authSecret); mac.Write([]byte(parts[0]))
  sig,err:=base64.RawURLEncoding.DecodeString(parts[1])
  if err!=nil || !hmac.Equal(sig,mac.Sum(nil)) { writeError(w,401,"invalid admin token"); return }
  raw,err:=base64.RawURLEncoding.DecodeString(parts[0]); if err!=nil {writeError(w,401,"invalid admin token");return}
  fields:=strings.Split(string(raw),"|")
  if len(fields)!=2 {writeError(w,401,"invalid admin token");return}
  exp,err:=strconv.ParseInt(fields[1],10,64)
  if err!=nil || time.Now().Unix()>exp {writeError(w,401,"admin token expired");return}
  next.ServeHTTP(w,r)
 })
}

