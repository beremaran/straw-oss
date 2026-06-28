# Contributing to Straw Proxy

Thank you for your interest in contributing to the Straw Proxy project. Please read through this guide to understand our development workflow, style guidelines, and review process.

---

## 🛠️ Development Workflow

1. **Find a Task**: Look at the issue tracker or project boards for available tasks.
2. **Create a Branch**: Use a descriptive name for your branch:
    * Features: `feature/description`
    * Bugfixes: `bugfix/issue-number-description`
    * Chore/Refactor: `chore/description`
3. **Make Changes**: Write clean, idiomatic Go code.
4. **Test**: Ensure all unit and integration tests pass before submitting.
   ```bash
   make test
   make lint
   ```
5. **Submit PR**: Create a Pull Request targeting the `main` branch.

---

## 📝 Code Style Guidelines

* **Formatting**: We enforce standard Go formatting (`gofmt`).
* **Linter**: We enforce strict code quality using `golangci-lint` configurations defined in `.golangci.yml`.
* **Variable Naming**: Follow Go standard practices (CamelCase for exported, camelCase for unexported).
* **Comments**: Document public methods, structures, and package interfaces. Explain the *why*, not just the *what*.

---

## 💬 Commit Messages

Please follow the [Conventional Commits](https://www.conventionalcommits.org/) specification:

* `feat: add new routing logic`
* `fix: resolve race condition in session manager`
* `docs: update architecture diagram`
* `chore: upgrade dependencies`

---

## 🔍 Pull Request Process

* **Description**: Clearly explain the changes and the problem they solve.
* **Context**: Link to relevant tasks, issues, or design documents.
* **Verification**: Describe how you verified the changes (e.g. manual curl outputs, new unit tests).
* **Approval**: All code changes require at least one approval from a repository maintainer before merging.
* **CI/CD**: CI checks must pass completely before merging.
