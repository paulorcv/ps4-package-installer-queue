# Configurando o NAS (WD My Cloud)

Passo a passo testado em um **WD My Cloud de primeira geração** (Debian 7, Apache 2.4, ARMv7). A ideia é subir uma segunda instância do Apache, separada da usada pelo painel da WD, servindo só a pasta dos `.pkg`.

## 1. Acesso SSH

Ative o SSH no painel do My Cloud. Como o firmware só oferece chaves `ssh-rsa`, o OpenSSH moderno precisa liberar esse algoritmo. Adicione ao `~/.ssh/config`:

```
Host mycloud
    HostName 192.168.0.121
    User root
    HostKeyAlgorithms +ssh-rsa
    PubkeyAcceptedAlgorithms +ssh-rsa
```

Sem o arquivo de config, o comando equivalente é:

```powershell
ssh -o HostKeyAlgorithms=+ssh-rsa -o PubkeyAcceptedAlgorithms=+ssh-rsa root@192.168.0.121
```

Conecte uma vez e aceite a chave. Isso registra o NAS no `known_hosts`, que o ps4pkgq usa para validar a conexão.

O ps4pkgq já faz o equivalente às duas opções acima, sem configuração extra: aceita chave de servidor `ssh-rsa` (e `ssh-dss`) e a troca de chaves `diffie-hellman-group14-sha1`, e no login por chave RSA assina com `ssh-rsa` quando o servidor não anuncia algoritmos mais novos. Isso é coberto por `internal/monitor/watcher_test.go`, que sobe um servidor SSH limitado a esses algoritmos.

## 2. Configuração do Apache

Crie `/root/pkg-httpd.conf` apontando para a pasta dos pacotes:

```apache
ServerRoot "/etc/apache2"
ServerName pkgserver
Listen 8080
PidFile /var/run/apache2-pkg.pid
User www-data
Group www-data

LoadModule mpm_prefork_module /usr/lib/apache2/modules/mod_mpm_prefork.so
LoadModule authz_core_module  /usr/lib/apache2/modules/mod_authz_core.so
LoadModule mime_module        /usr/lib/apache2/modules/mod_mime.so
LoadModule dir_module         /usr/lib/apache2/modules/mod_dir.so
LoadModule autoindex_module   /usr/lib/apache2/modules/mod_autoindex.so

StartServers          1
MinSpareServers       1
MaxSpareServers       2
MaxRequestWorkers     6

TypesConfig /etc/mime.types
AddType application/octet-stream .pkg

DocumentRoot "/shares/paulo/ps4"
<Directory "/shares/paulo/ps4">
    Options Indexes
    AllowOverride None
    Require all granted
</Directory>

ErrorLog /var/log/apache2/pkg-error.log

# Usado pelo ps4pkgq para saber o que foi servido ao PS4
LogFormat "%h %t \"%r\" %>s %B" pkglog
CustomLog /var/log/apache2/pkg-access.log pkglog
```

Valide e suba:

```bash
apache2 -f /root/pkg-httpd.conf -t
apache2 -f /root/pkg-httpd.conf -k start
```

Teste do PC. A resposta precisa ser `206 Partial Content`, porque o PS4 baixa por faixas (Range):

```powershell
curl.exe -I -H "Range: bytes=0-99" http://192.168.0.121:8080/jogo.pkg
```

> O servidor não tem senha. Sirva apenas a pasta dos pacotes, nunca a sua pasta pessoal inteira.

## 3. Subir junto com o NAS

O firmware da WD não executa o `rc.local` por padrão. O script abaixo espera o volume de dados montar antes de iniciar o Apache:

```bash
cat > /root/start-pkgserver.sh << 'EOF'
#!/bin/sh
for i in $(seq 1 60); do
    [ -d /shares/paulo/ps4 ] && break
    sleep 2
done
mkdir -p /var/log/apache2 /var/run
/usr/sbin/apache2 -f /root/pkg-httpd.conf -k start >> /root/pkgserver-boot.log 2>&1
echo "$(date) start exit=$?" >> /root/pkgserver-boot.log
EOF
chmod +x /root/start-pkgserver.sh

cat > /etc/rc.local << 'EOF'
#!/bin/sh -e
/root/start-pkgserver.sh &
exit 0
EOF
chmod +x /etc/rc.local
update-rc.d rc.local defaults
```

Depois de um `reboot`, confira com `cat /root/pkgserver-boot.log`. O esperado é `start exit=0`.

## 4. Backup da configuração

Atualizações de firmware podem apagar o `/root`. Guarde uma cópia no volume de dados:

```bash
cp /root/pkg-httpd.conf /root/start-pkgserver.sh /shares/paulo/
```

E desative a atualização automática de firmware no painel do My Cloud.
