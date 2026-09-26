# A small dedicated VPC with one public subnet. The instance needs a public
# address to serve the API and to reach Frankfurter, ECR and SSM.

resource "aws_vpc" "main" {
  cidr_block           = var.vpc_cidr
  enable_dns_support   = true
  enable_dns_hostnames = true

  tags = { Name = "${local.name}-vpc" }
}

resource "aws_internet_gateway" "main" {
  vpc_id = aws_vpc.main.id

  tags = { Name = "${local.name}-igw" }
}

resource "aws_subnet" "public" {
  vpc_id            = aws_vpc.main.id
  cidr_block        = var.public_subnet_cidr
  availability_zone = data.aws_availability_zones.available.names[0]

  tags = { Name = "${local.name}-public" }
}

resource "aws_route_table" "public" {
  vpc_id = aws_vpc.main.id

  route {
    cidr_block = "0.0.0.0/0"
    gateway_id = aws_internet_gateway.main.id
  }

  tags = { Name = "${local.name}-public" }
}

resource "aws_route_table_association" "public" {
  subnet_id      = aws_subnet.public.id
  route_table_id = aws_route_table.public.id
}

# Only the API port is open. There is deliberately no SSH rule: the instance is
# managed through SSM, so port 22 stays closed.
resource "aws_security_group" "api" {
  name        = "${local.name}-api"
  description = "Currency Watcher API: HTTP in, all traffic out"
  vpc_id      = aws_vpc.main.id

  tags = { Name = "${local.name}-api" }
}

resource "aws_vpc_security_group_ingress_rule" "http" {
  for_each = toset(var.allowed_ingress_cidrs)

  security_group_id = aws_security_group.api.id
  description       = "API traffic"
  ip_protocol       = "tcp"
  from_port         = var.host_port
  to_port           = var.host_port
  cidr_ipv4         = each.value
}

resource "aws_vpc_security_group_egress_rule" "all" {
  security_group_id = aws_security_group.api.id
  description       = "Outbound to Frankfurter, ECR, SSM and package repos"
  ip_protocol       = "-1"
  cidr_ipv4         = "0.0.0.0/0"
}
