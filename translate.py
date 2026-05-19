import os
import re

replacements = {
    r">Nome<": ">Name<",
    r">Fabricante<": ">Vendor<",
    r">Credenciais<": ">Credentials<",
    r">Rotinas<": ">Routines<",
    r">Configurações<": ">Settings<",
    r">Sistema<": ">System<",
    r">Usuários<": ">Users<",
    r">Usuário<": ">User<",
    r">Senha<": ">Password<",
    r">Salvar<": ">Save<",
    r">Sair<": ">Logout<",
    r">Voltar<": ">Back<",
    r">Editar<": ">Edit<",
    r">Excluir<": ">Delete<",
    r">Novo Node<": ">New Node<",
    r">Nova Credencial<": ">New Credential<",
    r">Novo Usuário<": ">New User<",
    r">Nova Rotina<": ">New Routine<",
    r">Ativo<": ">Active<",
    r">Inativo<": ">Inactive<",
    r">Sucesso<": ">Success<",
    r">Falha<": ">Failed<",
    r">Carregando...<": ">Loading...<",
    r">Nenhum node em estado de falha!<": ">No failed nodes!<",
    r">Nenhum backup realizado ainda.<": ">No backups yet.<",
    r">Nenhuma atividade registrada.<": ">No recent activity.<",
    r">Próximos Backups<": ">Upcoming Backups<",
    r">Últimos Backups<": ">Recent Backups<",
    r">Atividade Recente<": ">Recent Activity<",
    r">Nodes em Falha<": ">Failed Nodes<",
    r">Total de Nodes<": ">Total Nodes<",
    r">Nodes Ativos<": ">Active Nodes<",
    r">Backups Realizados<": ">Total Backups<",
    r">Falhas<": ">Failures<",
    r">Detalhes do Node<": ">Node Details<",
    r">Frequência<": ">Frequency<",
    r">Horário<": ">Time<",
    r">Todos<": ">All<",
    r">Exportar<": ">Export<",
    r">Meu Perfil<": ">My Profile<",
    r">Logs do Sistema<": ">System Logs<",
    r"value=\"Salvar\"": "value=\"Save\"",
    r"value=\"Entrar\"": "value=\"Sign In\"",
    r"value=\"Criar\"": "value=\"Create\"",
    r"placeholder=\"Buscar...\"": "placeholder=\"Search...\"",
    r"placeholder=\"Senha\"": "placeholder=\"Password\"",
    r"placeholder=\"Usuário\"": "placeholder=\"Username\"",
    r">Entrar<": ">Sign In<",
    r">Criar Conta<": ">Create Account<",
    r">Confirmar Exclusão<": ">Confirm Deletion<",
    r">Tem certeza que deseja excluir<": ">Are you sure you want to delete<",
    r">Esta ação não pode ser desfeita.<": ">This action cannot be undone.<",
    r">Cancelar<": ">Cancel<",
    r">Sim, Excluir<": ">Yes, Delete<",
}

def translate_file(filepath):
    with open(filepath, 'r', encoding='utf-8') as f:
        content = f.read()
        
    original = content
    for pt, en in replacements.items():
        content = re.sub(pt, en, content)
        
    if content != original:
        with open(filepath, 'w', encoding='utf-8') as f:
            f.write(content)
        print(f"Translated: {filepath}")

for root, _, files in os.walk('c:/mimic_backup/templates'):
    for file in files:
        if file.endswith('.html'):
            translate_file(os.path.join(root, file))
