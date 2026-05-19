package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

var replacements = map[string]string{
	"Nenhuma credencial cadastrada.": "No credentials registered.",
	"Nenhum node em estado de falha! 🎉": "No failed nodes! 🎉",
	"Nenhum backup realizado ainda.": "No backups performed yet.",
	"Nenhum backup agendado no momento.": "No scheduled backups at the moment.",
	"Nenhuma atividade registrada.": "No recorded activity.",
	"Nenhum node cadastrado.": "No nodes registered.",
	"Nenhuma exportação realizada": "No exports performed",
	"Nenhum node ativo.": "No active nodes.",
	"Nenhum log registrado.": "No logs recorded.",
	"Nenhuma rotina cadastrada.": "No routines registered.",
	"Nenhum usuário cadastrado.": "No users registered.",
    "title=\"Detalhes\"": "title=\"Details\"",
    "title=\"Editar\"": "title=\"Edit\"",
    "title=\"Excluir\"": "title=\"Delete\"",
    "title=\"Exportar\"": "title=\"Export\"",
    "title=\"Forçar Backup\"": "title=\"Force Backup\"",
    " title=\"Excluir\"": " title=\"Delete\"",
    ">Exportar Agora<": ">Export Now<",
    ">Sincronizar Todos<": ">Sync All<",
    ">Salvar Configurações<": ">Save Settings<",
    ">Detalhes<": ">Details<",
    ">Voltar para Dashboard<": ">Back to Dashboard<",
    ">Status Atual<": ">Current Status<",
    ">IP/Host<": ">IP/Host<",
    ">Agendamento<": ">Schedule<",
    ">Último Backup<": ">Last Backup<",
    ">Ações<": ">Actions<",
    ">Adicionar Node<": ">Add Node<",
    ">Importar CSV<": ">Import CSV<",
    ">Exportar CSV<": ">Export CSV<",
    ">Criar Nova Rotina<": ">Create New Routine<",
    ">Adicionar Usuário<": ">Add User<",
    ">Adicionar Credencial<": ">Add Credential<",
    ">Nome da Credencial<": ">Credential Name<",
    ">Selecione uma Rotina<": ">Select a Routine<",
    ">Nenhuma Rotina<": ">No Routine<",
    ">Selecione uma Credencial<": ">Select a Credential<",
    ">Credencial Customizada<": ">Custom Credential<",
    ">Sim<": ">Yes<",
    ">Não<": ">No<",
    ">Todos os Dias<": ">Every Day<",
    ">Dias da Semana<": ">Weekdays<",
    ">Finais de Semana<": ">Weekends<",
    ">Diariamente<": ">Daily<",
    ">Semanalmente<": ">Weekly<",
    ">Mensalmente<": ">Monthly<",
    ">Visualizar Backup<": ">View Backup<",
    ">Nível<": ">Level<",
    ">Categoria<": ">Category<",
    ">Mensagem<": ">Message<",
    ">Data/Hora<": ">Date/Time<",
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
