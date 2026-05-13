# Guia de Estilo e Identidade Visual — Mimic Backup

Este arquivo serve como referência para futuras sessões do Antigravity ou outros desenvolvedores manterem a consistência visual do projeto.

## 🎨 Identidade Visual

### Arquivos de Imagem
Todos os ativos visuais estão localizados em: `static/img/`

- **`favicon.ico`**: Ícone da aba do navegador.
- **`logo.svg`**: Logo colorido (ícone apenas). Usado em páginas com fundo claro ou quando se deseja destaque colorido.
- **`logo_branco.svg`**: Versão minimalista branca do ícone. Ideal para o menu lateral (sidebar) que possui fundo escuro.
- **`logo_texto.svg`**: Versão do logo que já inclui a tipografia/texto da marca. Usado centralizado nas páginas de Login e Setup.

### Aplicação nos Templates
- **Páginas de Autenticação (`login.html`, `setup_*.html`)**:
    - Usam `logo_texto.svg` centralizado.
    - Altura sugerida: `180px`.
    - O texto "Mimic" em HTML foi removido para usar a tipografia da própria imagem.
- **Painel Interno (`base.html`)**:
    - Usa `logo.svg` (ou `logo_branco.svg`) no topo da barra lateral.
    - Altura sugerida: `28px`.
    - Mantém o texto "Mimic" ao lado do ícone para navegação.

## 🛠️ Configurações de Desenvolvimento

### Recarregamento de Templates
O motor de templates (GoFiber HTML) foi configurado no `cmd/mimic/main.go` com:
```go
engine.Reload(true)
```
Isso permite que alterações nos arquivos `.html` dentro da pasta `templates/` sejam refletidas instantaneamente no navegador ao atualizar a página (F5), sem necessidade de reiniciar o servidor Go.

## 🌑 Sistema de Design (CSS)
O projeto utiliza um **Tema Escuro (Dark Theme)** profissional.
- **Fundo Principal**: `#11141F`
- **Fundo Secundário (Sidebar/Cards)**: `#171B26`
- **Cor de Destaque (Accent)**: `#6C63A6` (Roxo/Lavanda)
- **Texto Principal**: `#F2F0E6` (Off-white)

---
*Documento gerado pelo Antigravity para persistência de diretrizes visuais.*

## 🚀 Integração com Figma

Ao trazer novos designs do Figma (especialmente para a página de Login), os seguintes requisitos técnicos devem ser observados para manter a funcionalidade do backend:

### Requisitos do Formulário
- **Tag Form**: Deve conter `action="/login"` e `method="POST"`.
- **Campo de Usuário**: A tag `<input>` deve ter `name="username"`.
- **Campo de Senha**: A tag `<input>` deve ter `name="password"`.

### Lógica de Template (Go/Fiber)
O HTML deve conter os seguintes placeholders para funcionar com o servidor:
- **Mensagens de Erro**:
  ```html
  {{ if .Error }}
      <div class="sua-classe-de-erro">
          {{ .Error }}
      </div>
  {{ end }}
  ```
- **Persistência de Usuário**: Usar `value="{{ .LoginUsername }}"` no input de usuário para que o nome não suma em caso de erro de senha.

### Ativos (Assets)
- Exportar imagens do Figma para `static/img/`.
- Exportar ícones preferencialmente em `.svg`.
- Adicionar o Favicon no `<head>`:
  ```html
  <link rel="icon" type="image/x-icon" href="/static/img/favicon.ico" />
  ```

