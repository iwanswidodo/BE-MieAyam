package main

import (
 "context"
 "database/sql"
 "encoding/json"
 "errors"
 "fmt"
 "io"
 "log"
 "net/http"
 "os"
 "path/filepath"
 "strconv"
 "strings"
 "time"

 _ "github.com/jackc/pgx/v5/stdlib"
)

type Product struct {
 ID int64 `json:"id"`
 Name string `json:"name"`
 Category string `json:"category"`
 Variant string `json:"variant,omitempty"`
 Description string `json:"description,omitempty"`
 Price int64 `json:"price"`
 ImageURL string `json:"imageUrl,omitempty"`
 Active bool `json:"active"`
 CreatedAt time.Time `json:"createdAt"`
 UpdatedAt time.Time `json:"updatedAt"`
}
type productInput struct {
 Name string `json:"name"`
 Category string `json:"category"`
 Variant string `json:"variant"`
 Description string `json:"description"`
 Price int64 `json:"price"`
 Active *bool `json:"active"`
}
type server struct { db *sql.DB; uploadDir string }

const productColumns = "id,name,category,variant,description,price,image_url,active,created_at,updated_at"

func main() {
 dsn:=os.Getenv("DATABASE_URL")
 if dsn=="" { log.Fatal("DATABASE_URL is required") }
 db,err:=sql.Open("pgx",dsn); if err!=nil { log.Fatal(err) }; defer db.Close()
 db.SetMaxOpenConns(10); db.SetMaxIdleConns(5); db.SetConnMaxLifetime(30*time.Minute)
 ctx,cancel:=context.WithTimeout(context.Background(),10*time.Second); defer cancel()
 if err=db.PingContext(ctx); err!=nil { log.Fatalf("connect database: %v",err) }
 if err=migrate(db); err!=nil { log.Fatalf("database migration: %v",err) }
 uploadDir:=envOr("UPLOAD_DIR","uploads")
 if err=os.MkdirAll(uploadDir,0755); err!=nil { log.Fatal(err) }
 s:=&server{db:db,uploadDir:uploadDir}
 mux:=http.NewServeMux()
 mux.HandleFunc("GET /api/health",s.health)
 mux.HandleFunc("GET /api/products",s.listProducts)
 mux.HandleFunc("POST /api/products",s.createProduct)
 mux.HandleFunc("GET /api/products/{id}",s.getProduct)
 mux.HandleFunc("PUT /api/products/{id}",s.updateProduct)
 mux.HandleFunc("DELETE /api/products/{id}",s.deleteProduct)
 mux.HandleFunc("POST /api/products/{id}/image",s.uploadProductImage)
 mux.Handle("/uploads/",http.StripPrefix("/uploads/",http.FileServer(http.Dir(uploadDir))))
 addr:=envOr("APP_ADDR",":8080")
 log.Printf("BE-MieAyam listening on %s",addr)
 log.Fatal(http.ListenAndServe(addr,cors(mux)))
}
func migrate(db *sql.DB) error {
 _,err:=db.Exec(`CREATE TABLE IF NOT EXISTS products (
 id BIGSERIAL PRIMARY KEY,
 name TEXT NOT NULL,
 category TEXT NOT NULL,
 variant TEXT NOT NULL DEFAULT '',
 description TEXT NOT NULL DEFAULT '',
 price BIGINT NOT NULL CHECK(price >= 0),
 image_url TEXT NOT NULL DEFAULT '',
 active BOOLEAN NOT NULL DEFAULT TRUE,
 created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
 );
 CREATE INDEX IF NOT EXISTS idx_products_active_category ON products(active,category);`)
 return err
}
func (s *server) health(w http.ResponseWriter,r *http.Request) {
 if err:=s.db.PingContext(r.Context()); err!=nil { writeError(w,503,"database unavailable"); return }
 writeJSON(w,200,map[string]string{"status":"UP","app":"BE-MieAyam","database":"UP"})
}
func (s *server) listProducts(w http.ResponseWriter,r *http.Request) {
 q:="SELECT "+productColumns+" FROM products"; args:=[]any{}; clauses:=[]string{}
 if r.URL.Query().Get("all")!="true" { clauses=append(clauses,"active=TRUE") }
 if c:=strings.TrimSpace(r.URL.Query().Get("category")); c!="" { args=append(args,c); clauses=append(clauses,fmt.Sprintf("category=$%d",len(args))) }
 if len(clauses)>0 { q+=" WHERE "+strings.Join(clauses," AND ") }
 q+=" ORDER BY category,name,variant,id"
 rows,err:=s.db.QueryContext(r.Context(),q,args...); if err!=nil { writeError(w,500,"could not load products"); return }; defer rows.Close()
 products:=[]Product{}
 for rows.Next() { p,e:=scanProduct(rows); if e!=nil { writeError(w,500,"could not read products"); return }; products=append(products,p) }
 if err=rows.Err(); err!=nil { writeError(w,500,"could not read products"); return }
 writeJSON(w,200,products)
}
func (s *server) getProduct(w http.ResponseWriter,r *http.Request) {
 id,ok:=productID(w,r); if !ok{return}
 p,err:=scanProduct(s.db.QueryRowContext(r.Context(),"SELECT "+productColumns+" FROM products WHERE id=$1",id))
 if errors.Is(err,sql.ErrNoRows) { writeError(w,404,"product not found"); return }; if err!=nil { writeError(w,500,"could not load product"); return }; writeJSON(w,200,p)
}
func (s *server) createProduct(w http.ResponseWriter,r *http.Request) {
 var in productInput; if decodeJSON(r,&in)!=nil { writeError(w,400,"invalid JSON body"); return }; if validate(in)!=nil { writeError(w,400,validate(in).Error()); return }
 active:=true; if in.Active!=nil {active=*in.Active}
 q:="INSERT INTO products(name,category,variant,description,price,active) VALUES($1,$2,$3,$4,$5,$6) RETURNING "+productColumns
 p,err:=scanProduct(s.db.QueryRowContext(r.Context(),q,strings.TrimSpace(in.Name),strings.TrimSpace(in.Category),strings.TrimSpace(in.Variant),strings.TrimSpace(in.Description),in.Price,active))
 if err!=nil {writeError(w,500,"could not create product");return}; writeJSON(w,201,p)
}
func (s *server) updateProduct(w http.ResponseWriter,r *http.Request) {
 id,ok:=productID(w,r);if !ok{return}
 var in productInput;if decodeJSON(r,&in)!=nil {writeError(w,400,"invalid JSON body");return};if e:=validate(in);e!=nil {writeError(w,400,e.Error());return}
 active:=true;if in.Active!=nil {active=*in.Active}
 q:="UPDATE products SET name=$1,category=$2,variant=$3,description=$4,price=$5,active=$6,updated_at=NOW() WHERE id=$7 RETURNING "+productColumns
 p,err:=scanProduct(s.db.QueryRowContext(r.Context(),q,strings.TrimSpace(in.Name),strings.TrimSpace(in.Category),strings.TrimSpace(in.Variant),strings.TrimSpace(in.Description),in.Price,active,id))
 if errors.Is(err,sql.ErrNoRows) {writeError(w,404,"product not found");return};if err!=nil {writeError(w,500,"could not update product");return};writeJSON(w,200,p)
}
func (s *server) deleteProduct(w http.ResponseWriter,r *http.Request) {
 id,ok:=productID(w,r);if !ok{return}
 res,err:=s.db.ExecContext(r.Context(),"UPDATE products SET active=FALSE,updated_at=NOW() WHERE id=$1",id);if err!=nil {writeError(w,500,"could not deactivate product");return}
 n,_:=res.RowsAffected();if n==0 {writeError(w,404,"product not found");return};w.WriteHeader(204)
}
func (s *server) uploadProductImage(w http.ResponseWriter,r *http.Request) {
 id,ok:=productID(w,r);if !ok{return}
 var exists int;err:=s.db.QueryRowContext(r.Context(),"SELECT 1 FROM products WHERE id=$1",id).Scan(&exists)
 if errors.Is(err,sql.ErrNoRows) {writeError(w,404,"product not found");return};if err!=nil {writeError(w,500,"could not find product");return}
 r.Body=http.MaxBytesReader(w,r.Body,5<<20);if err=r.ParseMultipartForm(5<<20);err!=nil {writeError(w,400,"image must be 5 MB or smaller");return}
 file,header,err:=r.FormFile("image");if err!=nil {writeError(w,400,"multipart field 'image' is required");return};defer file.Close()
 ext:=strings.ToLower(filepath.Ext(header.Filename));if ext!=".jpg"&&ext!=".jpeg"&&ext!=".png" {writeError(w,400,"only JPG and PNG images are supported");return}
 name:=fmt.Sprintf("product-%d-%d%s",id,time.Now().UnixNano(),ext);path:=filepath.Join(s.uploadDir,name)
 dst,err:=os.Create(path);if err!=nil {writeError(w,500,"could not save image");return}
 if _,err=io.Copy(dst,file);err!=nil {dst.Close();os.Remove(path);writeError(w,500,"could not save image");return};if err=dst.Close();err!=nil {os.Remove(path);writeError(w,500,"could not save image");return}
 imageURL:="/uploads/"+name
 if _,err=s.db.ExecContext(r.Context(),"UPDATE products SET image_url=$1,updated_at=NOW() WHERE id=$2",imageURL,id);err!=nil {os.Remove(path);writeError(w,500,"could not attach image");return}
 writeJSON(w,200,map[string]string{"imageUrl":imageURL})
}
type scanner interface{Scan(...any)error}
func scanProduct(row scanner)(Product,error){var p Product;err:=row.Scan(&p.ID,&p.Name,&p.Category,&p.Variant,&p.Description,&p.Price,&p.ImageURL,&p.Active,&p.CreatedAt,&p.UpdatedAt);return p,err}
func validate(p productInput)error{if strings.TrimSpace(p.Name)==""{return errors.New("name is required")};if strings.TrimSpace(p.Category)==""{return errors.New("category is required")};if p.Price<0{return errors.New("price cannot be negative")};return nil}
func productID(w http.ResponseWriter,r *http.Request)(int64,bool){id,e:=strconv.ParseInt(r.PathValue("id"),10,64);if e!=nil||id<1{writeError(w,400,"invalid product id");return 0,false};return id,true}
func decodeJSON(r *http.Request,v any)error{defer r.Body.Close();return json.NewDecoder(r.Body).Decode(v)}
func writeJSON(w http.ResponseWriter,status int,v any){w.Header().Set("Content-Type","application/json");w.WriteHeader(status);_=json.NewEncoder(w).Encode(v)}
func writeError(w http.ResponseWriter,status int,msg string){writeJSON(w,status,map[string]string{"error":msg})}
func envOr(k,d string)string{if v:=os.Getenv(k);v!=""{return v};return d}
func cors(next http.Handler)http.Handler{return http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){origin:=os.Getenv("CORS_ORIGIN");if origin!=""{w.Header().Set("Access-Control-Allow-Origin",origin)};w.Header().Set("Access-Control-Allow-Methods","GET,POST,PUT,DELETE,OPTIONS");w.Header().Set("Access-Control-Allow-Headers","Content-Type,Authorization");if r.Method=="OPTIONS"{w.WriteHeader(204);return};next.ServeHTTP(w,r)})}
