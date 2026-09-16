// Команда staticlint запускает multichecker: анализаторы go vet, проверки
// staticcheck.io, bodyclose, nilerr и собственный osexitcheck.
//
// # Запуск
//
//	go install ./cmd/staticlint
//	staticlint ./...
//	go vet -vettool=$(which staticlint) ./...
//
// Набор проверок staticcheck.io задаёт config.json: он вшивается в бинарь при
// сборке через go:embed, поэтому инструмент работает из любого каталога. Поле
// prefixes включает классы по префиксу имени, exclude выключает отдельные
// проверки:
//
//	{"prefixes": ["SA", "S1", "ST1", "QF1"], "exclude": ["QF1008"]}
//
// Справка по конкретной проверке: staticlint help [имя].
//
// # Анализаторы golang.org/x/tools
//
// go tool vet help.
//
// # Анализаторы staticcheck.io
//
//	SA   ошибки и подозрительные конструкции, включены все проверки класса
//	S1   упрощение кода
//	ST1  оформление и именование
//	QF1  автоисправления gopls, кроме QF1008
//
// # Сторонние анализаторы
//
//	bodyclose  незакрытое тело HTTP-ответа
//	nilerr     возврат nil при ненулевой ошибке
//
// # Собственный анализатор
//
//	osexitcheck  запрещает прямой вызов os.Exit в функции main пакета main
package main
