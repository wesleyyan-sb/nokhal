# Nokhal

**Nokhal** is an embedded database for Go focused on **API simplicity** and **robust internal architecture**.
No SQL, no complex setup — just a single `.nok` file that is secure, fast, and easy to use.

---

## 🚀 Features

* ⚡ **High performance** with append-only (WAL) writes
* 🔒 **AES-256-GCM encryption** by default
* 🧠 **In-memory index** for fast reads
* 🔄 **Safe concurrency** (multi-goroutine)
* 💾 **Efficient binary storage** (no JSON)
* 🧩 **Simple, idiomatic Go API**
* 🛡️ **Crash-safe design** (no data corruption)
* 📦 **Single file storage (.nok)**

---

## 📦 Installation

```bash
go get github.com/wesleyyan-sb/Nokhal
```

---

## ⚡ Quick Start

```go
package main

import (
    "fmt"
    "github.com/wesleyyan-sb/nokhal"
)

type User struct {
    Name string
    Age  int
}

func main() {
    db, _ := nokhal.Open("data.nok", nil)

    user := User{Name: "Yan", Age: 17}

    db.Put("user:1", user)

    var result User
    db.Get("user:1", &result)

    fmt.Println(result.Name) // Yan
}
```

---

## 🧠 Concept

Nokhal is designed to be:

* As simple as using a `map`
* As safe as a real database
* As lightweight as a local file

It takes conceptual inspiration from SQLite, BoltDB, and BadgerDB — but with a strong focus on developer experience.

---

## 🔐 Security

* AES-256 in GCM mode
* Unique nonce per record
* Secure key derivation (Argon2 / PBKDF2)
* Transparent encryption at rest

> No custom cryptography is used.

---

## 🏗️ Architecture

* 📁 `.nok` file format
* 🧾 Append-only log (WAL)
* 🔁 Periodic compaction
* 📊 In-memory index rebuilt at startup
* ✅ Per-record checksum

---

## ⚙️ API

```go
db.Put(key, value)
db.Get(key, &value)
db.Delete(key)
db.Query(fn)
```

No SQL. No ORM. No overhead.

---

## 📈 Performance

Nokhal is designed for:

* Low latency
* Fast writes (append-only)
* Minimal memory allocations

Benchmarks coming soon.

---

## 🛣️ Roadmap

* [ ] Persistent indexing
* [ ] TTL (expiration)
* [ ] Snapshots
* [ ] Backup / Restore
* [ ] Full multi-process support

---

## 🤝 Contributing

Pull requests are welcome.
If you want to help build something simple, fast, and reliable — you're welcome here.

---

## 📜 License

Apache 2.0

---

## ✨ Philosophy

> Simple on the surface. Powerful underneath.
