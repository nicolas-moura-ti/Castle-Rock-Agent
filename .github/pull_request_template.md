## 📋 Descrição

Descreva de forma clara e concisa as mudanças introduzidas por este Pull Request e o contexto da alteração.

Fixes #(número da issue) <!-- ou Closes #... se fechar uma issue -->

---

## 🛠 Tipo de Mudança

Marque as opções aplicáveis com um `x`:

- [ ] 🐛 **Correção de Bug** (mudança que não quebra compatibilidade e corrige um problema)
- [ ] ✨ **Nova Funcionalidade** (mudança que adiciona um novo recurso sem quebrar compatibilidade)
- [ ] 💥 **Breaking Change** (mudança que altera comportamentos anteriores ou quebra compatibilidade de API/config)
- [ ] 📝 **Documentação** (mudança apenas em arquivos de documentação)
- [ ] 🧹 **Refatoração / Manutenção** (melhorias de código, performance, formatação ou testes)

---

## 🧪 Como Testar

Instruções para testar as mudanças localmente:

```bash
# Exemplo:
make test
make lint
make build
```

---

## ✅ Checklist

- [ ] Meu código segue os padrões do projeto e convenções de Go (`gofmt`, `golangci-lint`).
- [ ] Executei a suíte de testes localmente e todos os testes passaram (`make test`).
- [ ] Adicionei testes unitários cobrindo o novo código/correção (se aplicável).
- [ ] Atualizei a documentação relevante (`README.md`, `README.pt-BR.md`, docs) se necessário.
- [ ] A mensagem de commit está clara e bem descritiva.
