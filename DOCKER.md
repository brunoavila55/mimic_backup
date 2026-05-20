# Instalação do Mimic Backup com Docker

A instalação via Docker é a forma mais rápida e recomendada para ambientes modernos, pois já empacota o banco de dados PostgreSQL e a aplicação pré-configurados em contêineres isolados.

## Pré-requisitos
- [Docker](https://docs.docker.com/get-docker/) instalado no servidor.
- [Docker Compose](https://docs.docker.com/compose/install/) instalado (ou `docker compose` em versões mais recentes do Docker).

---

## 1. Clonar o Repositório
Baixe o código para o servidor onde deseja rodar:

```bash
git clone https://github.com/brunoavila55/mimic_backup.git
cd mimic_backup
```

## 2. Configurações Opcionais (Variáveis de Ambiente)
O arquivo `docker-compose.yml` já vem com todas as credenciais e endereços apontados corretamente para o contêiner do banco de dados (chamado `db`). 

Caso queira alterar a **senha do banco** ou a **SECRET_KEY** da aplicação, edite o arquivo `docker-compose.yml` usando qualquer editor (como `nano` ou `vim`) e altere os valores na seção `environment`.

> **Recomendação de Segurança:** A `SECRET_KEY` padrão deve ser alterada em ambientes de produção. Basta substituir `chave_super_secreta_padrao_para_docker` por qualquer frase aleatória de 32 caracteres.

## 3. Subir os Contêineres
Estando dentro da pasta `mimic_backup`, execute o comando:

```bash
docker compose up --build -d
```
*Observação: Se você estiver usando uma versão antiga do Docker, o comando pode ser `docker-compose up --build -d` (com hífen).*

O Docker fará o download da imagem do Postgres, compilará o código do Mimic localmente (graças ao nosso `Dockerfile` de múltiplos estágios) e iniciará ambos os contêineres. O `-d` garante que os contêineres rodem em segundo plano (detached mode).

## 4. Verificar os Logs
Se você quiser ter certeza de que o sistema e o banco de dados iniciaram corretamente, olhe os logs:
```bash
docker compose logs -f
```
Para sair dos logs, pressione `Ctrl+C`.

## 5. Acesso
Tudo pronto! Acesse no seu navegador `http://localhost:3000` (ou troque o `localhost` pelo IP do seu servidor). Na primeira tela, você passará pelo **First Setup** para criar seu primeiro administrador.

## Atualização (Redeploy)
Quando sair uma versão nova do Mimic e você quiser atualizar, basta fazer o processo na mesma pasta:
```bash
git pull
docker compose down
docker compose up --build -d
```
Seu banco de dados não será perdido, pois os dados estão persistidos em um Volume do Docker configurado no `docker-compose.yml`.
