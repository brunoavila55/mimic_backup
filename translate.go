package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

var replacements = map[string]string{
	">Nome<": ">Name<",
	">Fabricante<": ">Vendor<",
	">Credenciais<": ">Credentials<",
	">Rotinas<": ">Routines<",
	">Configurações<": ">Settings<",
	">Sistema<": ">System<",
	">Usuários<": ">Users<",
	">Usuário<": ">User<",
	">Senha<": ">Password<",
	">Salvar<": ">Save<",
	">Sair<": ">Logout<",
	">Voltar<": ">Back<",
	">Editar<": ">Edit<",
	">Excluir<": ">Delete<",
	">Novo Node<": ">New Node<",
	">Nova Credencial<": ">New Credential<",
	">Novo Usuário<": ">New User<",
	">Nova Rotina<": ">New Routine<",
	">Ativo<": ">Active<",
	">Inativo<": ">Inactive<",
	">Sucesso<": ">Success<",
	">Falha<": ">Failed<",
	">Carregando...<": ">Loading...<",
	">Nenhum node em estado de falha!<": ">No failed nodes!<",
	">Nenhum backup realizado ainda.<": ">No backups yet.<",
	">Nenhuma atividade registrada.<": ">No recent activity.<",
	">Próximos Backups<": ">Upcoming Backups<",
	">Últimos Backups<": ">Recent Backups<",
	">Atividade Recente<": ">Recent Activity<",
	">Nodes em Falha<": ">Failed Nodes<",
	">Total de Nodes<": ">Total Nodes<",
	">Nodes Ativos<": ">Active Nodes<",
	">Backups Realizados<": ">Total Backups<",
	">Falhas<": ">Failures<",
	">Detalhes do Node<": ">Node Details<",
	">Frequência<": ">Frequency<",
	">Horário<": ">Time<",
	">Todos<": ">All<",
	">Exportar<": ">Export<",
	">Meu Perfil<": ">My Profile<",
	">Logs do Sistema<": ">System Logs<",
	"value=\"Salvar\"": "value=\"Save\"",
	"value=\"Entrar\"": "value=\"Sign In\"",
	"value=\"Criar\"": "value=\"Create\"",
	"placeholder=\"Buscar...\"": "placeholder=\"Search...\"",
	"placeholder=\"Senha\"": "placeholder=\"Password\"",
	"placeholder=\"Usuário\"": "placeholder=\"Username\"",
	">Entrar<": ">Sign In<",
	">Criar Conta<": ">Create Account<",
	">Confirmar Exclusão<": ">Confirm Deletion<",
	">Tem certeza que deseja excluir<": ">Are you sure you want to delete<",
	">Esta ação não pode ser desfeita.<": ">This action cannot be undone.<",
	">Cancelar<": ">Cancel<",
	">Sim, Excluir<": ">Yes, Delete<",
    // Base/HTMX specific translations
    "Sair do sistema": "Logout",
    "Carregando": "Loading",
}

func main() {
	err := filepath.WalkDir("templates", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".html") {
			return nil
		}

		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		content := string(b)
		original := content

		for pt, en := range replacements {
			content = strings.ReplaceAll(content, pt, en)
		}

		if content != original {
			err = os.WriteFile(path, []byte(content), 0644)
			if err != nil {
				return err
			}
			fmt.Printf("Translated: %s\n", path)
		}

		return nil
	})

	if err != nil {
		fmt.Println("Error:", err)
	}
}
