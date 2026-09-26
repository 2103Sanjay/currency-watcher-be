# ---- Instance role: lets the host pull images and be managed through SSM ------

data "aws_iam_policy_document" "ec2_assume_role" {
  statement {
    actions = ["sts:AssumeRole"]
    principals {
      type        = "Service"
      identifiers = ["ec2.amazonaws.com"]
    }
  }
}

resource "aws_iam_role" "instance" {
  name               = "${local.name}-instance"
  assume_role_policy = data.aws_iam_policy_document.ec2_assume_role.json
}

# Allows SSM Run Command (used by the deploy pipeline) and Session Manager.
resource "aws_iam_role_policy_attachment" "instance_ssm" {
  role       = aws_iam_role.instance.name
  policy_arn = "arn:aws:iam::aws:policy/AmazonSSMManagedInstanceCore"
}

# Allows the host to pull the API image from ECR.
resource "aws_iam_role_policy_attachment" "instance_ecr_read" {
  role       = aws_iam_role.instance.name
  policy_arn = "arn:aws:iam::aws:policy/AmazonEC2ContainerRegistryReadOnly"
}

resource "aws_iam_instance_profile" "instance" {
  name = "${local.name}-instance"
  role = aws_iam_role.instance.name
}

# ---- Instance -------------------------------------------------------------------

resource "aws_instance" "api" {
  ami                    = data.aws_ssm_parameter.al2023_ami.value
  instance_type          = var.instance_type
  subnet_id              = aws_subnet.public.id
  vpc_security_group_ids = [aws_security_group.api.id]
  iam_instance_profile   = aws_iam_instance_profile.instance.name

  # First-boot setup: install Docker, write the app configuration, and start
  # the latest released image if one exists.
  user_data = templatefile("${path.module}/templates/user_data.sh.tftpl", {
    aws_region      = var.aws_region
    image           = "${aws_ecr_repository.api.repository_url}:latest"
    host_port       = var.host_port
    app_port        = var.app_port
    allowed_origins = join(",", var.allowed_origins)
    cache_ttl       = var.cache_ttl
    deploy_script   = file("${path.module}/../scripts/deploy.sh")
  })
  # user_data only runs on first boot, so configuration changes need a new
  # instance. The Elastic IP moves over automatically; update the
  # EC2_INSTANCE_ID variable in GitHub afterwards.
  user_data_replace_on_change = true

  metadata_options {
    http_tokens                 = "required" # IMDSv2 only
    http_put_response_hop_limit = 1          # containers can't reach instance credentials
  }

  root_block_device {
    volume_type           = "gp3"
    volume_size           = var.root_volume_size_gb
    encrypted             = true
    delete_on_termination = true
  }

  tags = { Name = "${local.name}-api" }

  lifecycle {
    # A newer Amazon Linux release shouldn't silently replace the running host.
    ignore_changes = [ami]
  }
}

# A fixed public IP, so the API URL survives instance stops and replacements.
resource "aws_eip" "api" {
  domain   = "vpc"
  instance = aws_instance.api.id

  tags = { Name = "${local.name}-api" }

  depends_on = [aws_internet_gateway.main]
}
