#!/usr/bin/env bash
set -euo pipefail

REPO="Tejas163/TokenSentinel"
BRANCH="main"
COMPOSE_FILE="proxyops_gateway/docker-compose.yml"

GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
NC='\033[0m'

echo -e "${BLUE}╔══════════════════════════════════════╗${NC}"
echo -e "${BLUE}║      TokenSentinel Installer         ║${NC}"
echo -e "${BLUE}║  LLM Cost Governance Platform        ║${NC}"
echo -e "${BLUE}╚══════════════════════════════════════╝${NC}"
echo ""

if ! command -v docker &>/dev/null; then
    echo -e "${YELLOW}Docker not found. Installing Docker...${NC}"
    curl -fsSL https://get.docker.com | sh
    if [ "$(id -u)" -ne 0 ]; then
        echo "Adding user to docker group..."
        sudo usermod -aG docker "$USER" || true
    fi
fi

if ! command -v docker compose &>/dev/null && ! docker compose version &>/dev/null; then
    echo -e "${YELLOW}Docker Compose not found. Installing...${NC}"
    sudo apt-get update -qq && sudo apt-get install -y -qq docker-compose-plugin 2>/dev/null || \
    echo -e "${YELLOW}Please install Docker Compose manually: https://docs.docker.com/compose/install/${NC}"
fi

DIR="tokensentinel"
if [ -d "$DIR" ]; then
    echo -e "${YELLOW}Directory '$DIR' already exists. Pulling latest updates...${NC}"
    cd "$DIR"
    git pull origin "$BRANCH" 2>/dev/null || true
else
    echo -e "${GREEN}Cloning TokenSentinel...${NC}"
    git clone --depth 1 --branch "$BRANCH" "https://github.com/$REPO.git" "$DIR"
    cd "$DIR"
fi

echo -e "${GREEN}Starting TokenSentinel services...${NC}"
docker compose -f "$COMPOSE_FILE" up -d --build

echo ""
echo -e "${GREEN}╔══════════════════════════════════════╗${NC}"
echo -e "${GREEN}║  TokenSentinel is running!            ║${NC}"
echo -e "${GREEN}╠══════════════════════════════════════╣${NC}"
echo -e "${GREEN}║  Dashboard:  http://localhost:3001    ║${NC}"
echo -e "${GREEN}║  Proxy:      http://localhost:3000    ║${NC}"
echo -e "${GREEN}║  Router:     http://localhost:8080    ║${NC}"
echo -e "${GREEN}╚══════════════════════════════════════╝${NC}"
echo ""
echo -e "Run ${BLUE}docker compose -f $DIR/$COMPOSE_FILE logs -f${NC} to follow logs."
