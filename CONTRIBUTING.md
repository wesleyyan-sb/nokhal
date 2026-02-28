# Contributing to Nokhal

First off, thank you for considering contributing to Nokhal 🚀  
This project aims to be simple on the outside, but powerful and reliable under the hood — and contributions are a big part of that.

---

## 🧭 Philosophy

- Keep the **API simple**
- Keep the **core robust**
- Prefer **clarity over cleverness**
- Security and correctness come first

---

## 🛠️ Getting Started

1. Fork the repository  
2. Clone your fork:

```bash
git clone https://github.com/wesleyyan-sb/nokhal.git
cd nokhal
````

3. Install dependencies:

```bash
go mod tidy
```

4. Run tests:

```bash
go test ./...
```

---

## 🌱 Branching

* Create a new branch for each feature or fix:

```bash
git checkout -b feature/my-feature
```

---

## ✍️ Code Guidelines

* Follow idiomatic Go practices
* Keep functions small and focused
* Avoid unnecessary abstractions
* Write clear and readable code
* Document exported functions

---

## 🔒 Security Rules

* **Never implement custom cryptography**
* Use only Go standard libraries for crypto
* Be careful with concurrency and data races
* Validate all edge cases (especially I/O and file handling)

---

## 🧪 Testing

* Add tests for all new features
* Ensure all tests pass before submitting
* Cover edge cases and failure scenarios when possible

Run:

```bash
go test ./...
```

---

## ⚡ Performance

* Avoid unnecessary allocations
* Prefer efficient data structures
* Benchmark critical changes when relevant

---

## 📦 Commit Messages

Use clear and descriptive commit messages:

```
feat: add in-memory index
fix: prevent data race in write path
refactor: simplify storage layer
```

---

## 📥 Pull Requests

Before opening a PR:

* Ensure the code builds and tests pass
* Keep PRs focused and minimal
* Include a clear description of the change

PRs should include:

* What was changed
* Why it was changed
* Any trade-offs or considerations

---

## 💬 Discussions

If you're unsure about a change, open an issue first.
Good discussions lead to better design decisions.

---

## 🛣️ Areas to Contribute

* Storage engine improvements
* Indexing strategies
* Performance optimizations
* Testing & benchmarks
* Documentation

---

## 🙌 Final Note

Nokhal is being built to be **simple, fast, and reliable**.

If your contribution moves the project in that direction — you're doing it right.

Thanks for being part of it 💙
