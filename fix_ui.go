package main

import (
	"fmt"
	"os"
	"strings"
)

var replacements = map[string]string{
	"Configuração Inicial": "Initial Setup",
	"Configuração Assistida": "Assisted Setup",
	"Preparamos tudo para você começar em minutos.": "We set everything up for you to start in minutes.",
	"Integridade de Dados": "Data Integrity",
	"Verificamos tabelas e conexões automaticamente.": "We check tables and connections automatically.",
	"Banco de Dados": "Database",
	"Administrador": "Administrator",
	"Verificando a infraestrutura básica do sistema.": "Checking the basic system infrastructure.",
	"Conexão estabelecida": "Connection established",
	"Falha na conexão": "Connection failed",
	"Tabelas criadas": "Tables created",
	"Tabelas pendentes": "Tables pending",
	"Estrutura do sistema": "System structure",
	"Continuar": "Continue",
    "Criar Administrador": "Create Administrator",
    "Configure a conta principal do sistema.": "Configure the main system account.",
    "Criar Conta": "Create Account",
}

func translateFile(path string) error {
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
}

func main() {
	files := []string{
		"templates/setup_database.html",
		"templates/setup_superuser.html",
	}

	for _, file := range files {
		if err := translateFile(file); err != nil {
			fmt.Printf("Error processing %s: %v\n", file, err)
		}
	}
}
