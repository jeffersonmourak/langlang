use crate::ast::{SemExpr, SemExprUnaryOp, SemValue, AST};
use std::boxed::Box;

#[derive(Debug)]
pub enum Error {
    BacktrackError(usize, String),
    ParseIntError(String),
}

impl std::error::Error for Error {}

impl From<std::num::ParseIntError> for Error {
    fn from(e: std::num::ParseIntError) -> Self {
        Error::ParseIntError(e.to_string())
    }
}

impl std::fmt::Display for Error {
    fn fmt(&self, f: &mut std::fmt::Formatter) -> std::fmt::Result {
        match self {
            Error::BacktrackError(i, m) => write!(f, "Syntax Error: {}: {}", i, m),
            Error::ParseIntError(e) => write!(f, "{}", e),
        }
    }
}

pub struct Parser {
    cursor: usize,
    ffp: usize,
    source: Vec<char>,
}

type ParseFn<T> = fn(&mut Parser) -> Result<T, Error>;

impl Parser {
    pub fn new(s: &str) -> Self {
        return Parser {
            cursor: 0,
            ffp: 0,
            source: s.chars().collect(),
        };
    }

    /// Traverse the input string fed into the parser's constructor
    /// and return an AST node when parsing succeeds
    pub fn parse(&mut self) -> Result<AST, Error> {
        self.parse_grammar()
    }

    // GR: Grammar <- Spacing (Definition / LabelDefinition / SemAction)+ EndOfFile
    fn parse_grammar(&mut self) -> Result<AST, Error> {
        self.parse_spacing()?;
        let defs = self.one_or_more(|p| {
            p.choice(vec![
                |p| p.parse_label_definition(),
                |p| p.parse_semaction(),
                |p| p.parse_definition(),
            ])
        })?;
        self.parse_eof()?;
        Ok(AST::Grammar(defs))
    }

    // GR: Definition <- Identifier LEFTARROW Expression
    fn parse_definition(&mut self) -> Result<AST, Error> {
        let id = self.parse_identifier()?;
        self.expect('<')?;
        self.expect('-')?;
        self.parse_spacing()?;
        let expr = self.parse_expression()?;
        Ok(AST::Definition(id, Box::new(expr)))
    }

    // GR: LabelDefinition <- LABEL Identifier EQ Literal
    fn parse_label_definition(&mut self) -> Result<AST, Error> {
        self.expect_str("label")?;
        self.parse_spacing()?;
        let label = self.parse_identifier()?;
        self.expect('=')?;
        self.parse_spacing()?;
        let literal = self.parse_literal_string()?;
        Ok(AST::LabelDefinition(label, literal))
    }

    // GR: SemAction  <- SemExprPth RIGHTARROW SemExpr^semexpr
    fn parse_semaction(&mut self) -> Result<AST, Error> {
        let id = self.parse_identifier()?;
        self.expect('-')?;
        self.expect('>')?;
        self.parse_spacing()?;
        let expr = self.parse_sem_expr()?;
        Ok(AST::SemanticAction(id, Box::new(expr)))
    }

    // GR: SemExprPth <- Identifier (SLASH (Decimal / Identifier)^exprpath)*

    // GR: SemExpr    <- SemUnary / SemValue / SemCall / Identifier
    fn parse_sem_expr(&mut self) -> Result<SemExpr, Error> {
        self.choice(vec![
            |p| p.parse_sem_expr_parens(),
            |p| p.parse_sem_unary(),
            |p| p.parse_sem_val(),
            |p| p.parse_sem_call(),
            |p| p.parse_sem_id(),
        ])
    }

    // GR: SemExprParens <- OPEN SemExpr¹^semexpexpr CLOSE^semexpcl
    fn parse_sem_expr_parens(&mut self) -> Result<SemExpr, Error> {
        self.expect('(')?;
        let expr = self.parse_sem_expr()?;
        self.expect(')')?;
        Ok(expr)
    }

    // GR: SemUnary <- (MINUS / PLUS) SemExpr
    fn parse_sem_unary(&mut self) -> Result<SemExpr, Error> {
        let op = match self.choice(vec![|p| p.expect('-'), |p| p.expect('+')]) {
            Err(er) => return Err(er),
            Ok('-') => SemExprUnaryOp::Negative,
            Ok('+') => SemExprUnaryOp::Positive,
            _ => unreachable!(),
        };
        let expr = self.parse_sem_expr()?;
        Ok(SemExpr::UnaryOp(op, Box::new(expr)))
    }

    fn parse_sem_id(&mut self) -> Result<SemExpr, Error> {
        let id = self.parse_identifier()?;
        Ok(SemExpr::Identifier(id))
    }

    // SemCall    <- Identifier OPEN (SemExpr (COMMA SemExpr)*)? CLOSE^acfncl
    fn parse_sem_call(&mut self) -> Result<SemExpr, Error> {
        let name = self.parse_identifier()?;
        self.expect('(')?;
        self.parse_spacing()?;
        let mut params = vec![];
        let first = self.parse_sem_expr();
        if let Ok(v) = first {
            params.push(v);
            params.extend(self.zero_or_more(|p| {
                p.expect(',')?;
                p.parse_spacing()?;
                p.parse_sem_expr()
            })?);
        }
        self.expect(')')?;
        self.parse_spacing()?;
        Ok(SemExpr::Call(name, params))
    }

    // GR: SemValue   <- Number / Literal / Variable / SemDict / SemList
    fn parse_sem_val(&mut self) -> Result<SemExpr, Error> {
        Ok(SemExpr::Value(self.choice(vec![
            |p| p.parse_number(),
            |p| p.parse_bool(),
            |p| p.parse_sem_lit(),
            |p| p.parse_sem_var(),
            // |p| p.parse_sem_dict(),
            |p| p.parse_sem_list(),
        ])?))
    }

    fn parse_sem_lit(&mut self) -> Result<SemValue, Error> {
        match self.parse_literal()? {
            AST::Char(c) => Ok(SemValue::Char(c)),
            AST::String(s) => Ok(SemValue::String(s)),
            _ => unreachable!(),
        }
    }

    // GR: SemVar <- '%' [0-9]+ _
    fn parse_sem_var(&mut self) -> Result<SemValue, Error> {
        self.expect('%')?;
        let var = self.parse_decimal_number()?;
        self.parse_spacing()?;
        Ok(SemValue::Variable(var as usize))
    }

    // GR: Number      <- Hexadecimal / Decimal
    fn parse_number(&mut self) -> Result<SemValue, Error> {
        Ok(SemValue::Number(self.choice(vec![
            // order matters here: the decimal number rule matches
            // against '0' which is the prefix of the hexadecimal
            // number rule
            |p| p.parse_hexadecimal_number(),
            |p| p.parse_decimal_number(),
        ])?))
    }

    // GR: Bool      <- TRUE / FALSE
    fn parse_bool(&mut self) -> Result<SemValue, Error> {
        Ok(SemValue::Bool(self.choice(vec![
            |p| {
                p.expect_str("true")?;
                p.parse_spacing()?;
                Ok(true)
            },
            |p| {
                p.expect_str("false")?;
                p.parse_spacing()?;
                Ok(false)
            },
        ])?))
    }

    // GR: SemList     <- OPENC (SemExpr (COMMA SemExpr)*)? CLOSEC^aclistcl
    fn parse_sem_list(&mut self) -> Result<SemValue, Error> {
        self.expect('{')?;
        self.parse_spacing()?;
        let mut items = vec![];
        let first = self.parse_sem_expr();
        if let Ok(v) = first {
            items.push(v);
            items.extend(self.zero_or_more(|p| {
                p.expect(',')?;
                p.parse_spacing()?;
                p.parse_sem_expr()
            })?);
        }
        self.expect('}')?;
        self.parse_spacing()?;
        Ok(SemValue::List(items))
    }

    // GR: Decimal     <- ([1-9][0-9]* / '0') _
    fn parse_decimal_number(&mut self) -> Result<i64, Error> {
        let digits = self.choice(vec![
            |p| {
                let mut d = p.expect_range('1', '9')?.to_string();
                let tail: String = p
                    .zero_or_more(|p| p.expect_range('0', '9'))?
                    .into_iter()
                    .collect();
                d.push_str(&tail);
                Ok(d)
            },
            |p| Ok(p.expect('0')?.to_string()),
        ])?;
        self.parse_spacing()?;
        Ok(digits.parse::<i64>()?)
    }

    // GR: Hexadecimal <- '0x' [a-zA-Z0-9]+ _
    fn parse_hexadecimal_number(&mut self) -> Result<i64, Error> {
        self.expect_str("0x")?;
        let digits: String = self
            .one_or_more(|p| {
                p.choice(vec![
                    |p| p.expect_range('0', '9'),
                    |p| p.expect_range('a', 'f'),
                    |p| p.expect_range('A', 'F'),
                ])
            })?
            .into_iter()
            .collect();
        self.parse_spacing()?;
        Ok(i64::from_str_radix(&digits, 16)?)
    }

    // GR: Expression <- Sequence (SLASH Sequence)*
    fn parse_expression(&mut self) -> Result<AST, Error> {
        let first = self.parse_sequence()?;
        let mut choices = vec![first];
        choices.append(&mut self.zero_or_more(|p| {
            p.expect('/')?;
            p.parse_spacing()?;
            p.parse_sequence()
        })?);
        Ok(if choices.len() == 1 {
            choices.remove(0)
        } else {
            AST::Choice(choices)
        })
    }

    // GR: Sequence <- Prefix*
    fn parse_sequence(&mut self) -> Result<AST, Error> {
        let seq = self.zero_or_more(|p| p.parse_prefix())?;
        Ok(AST::Sequence(if seq.is_empty() {
            vec![AST::Empty]
        } else {
            seq
        }))
    }

    // GR: Prefix <- (AND / NOT)? Labeled
    fn parse_prefix(&mut self) -> Result<AST, Error> {
        let prefix = self.choice(vec![
            |p| {
                p.expect_str("&")?;
                p.parse_spacing()?;
                Ok("&")
            },
            |p| {
                p.expect_str("!")?;
                p.parse_spacing()?;
                Ok("!")
            },
            |_| Ok(""),
        ]);
        let labeled = self.parse_labeled()?;
        Ok(match prefix {
            Ok("&") => AST::And(Box::new(labeled)),
            Ok("!") => AST::Not(Box::new(labeled)),
            _ => labeled,
        })
    }

    // GR: Labeled <- Suffix Label?
    fn parse_labeled(&mut self) -> Result<AST, Error> {
        let suffix = self.parse_suffix()?;
        Ok(match self.parse_label() {
            Ok(label) => AST::Label(label, Box::new(suffix)),
            _ => suffix,
        })
    }

    // GR: Label   <- [^⇑] Identifier
    fn parse_label(&mut self) -> Result<String, Error> {
        self.choice(vec![|p| p.expect_str("^"), |p| p.expect_str("⇑")])?;
        self.parse_identifier()
    }

    // GR: Suffix  <- Primary (QUESTION / STAR / PLUS / Superscript)?
    fn parse_suffix(&mut self) -> Result<AST, Error> {
        let primary = self.parse_primary()?;
        let suffix = self.choice(vec![
            |p| {
                p.expect_str("?")?;
                p.parse_spacing()?;
                Ok("?".to_string())
            },
            |p| {
                p.expect_str("*")?;
                p.parse_spacing()?;
                Ok("*".to_string())
            },
            |p| {
                p.expect_str("+")?;
                p.parse_spacing()?;
                Ok("+".to_string())
            },
            |p| {
                let sup = p.parse_superscript()?;
                p.parse_spacing()?;
                Ok(sup)
            },
            |_| Ok("".to_string()),
        ])?;
        Ok(match suffix.as_ref() {
            "?" => AST::Optional(Box::new(primary)),
            "*" => AST::ZeroOrMore(Box::new(primary)),
            "+" => AST::OneOrMore(Box::new(primary)),
            "¹" => AST::Precedence(Box::new(primary), 1),
            "²" => AST::Precedence(Box::new(primary), 2),
            "³" => AST::Precedence(Box::new(primary), 3),
            "⁴" => AST::Precedence(Box::new(primary), 4),
            "⁵" => AST::Precedence(Box::new(primary), 5),
            "⁶" => AST::Precedence(Box::new(primary), 6),
            "⁷" => AST::Precedence(Box::new(primary), 7),
            "⁸" => AST::Precedence(Box::new(primary), 8),
            "⁹" => AST::Precedence(Box::new(primary), 9),
            _ => primary,
        })
    }

    // GR: Superscript <- [¹-⁹]
    fn parse_superscript(&mut self) -> Result<String, Error> {
        self.choice(vec![
            |p| p.expect_str("¹"),
            |p| p.expect_str("²"),
            |p| p.expect_str("³"),
            |p| p.expect_str("⁴"),
            |p| p.expect_str("⁵"),
            |p| p.expect_str("⁶"),
            |p| p.expect_str("⁷"),
            |p| p.expect_str("⁸"),
            |p| p.expect_str("⁹"),
        ])
    }

    // GR: Primary <- Identifier !(LEFTARROW / (Identifier EQ))
    // GR:          / OPEN Expression CLOSE
    // GR:          / Node / List / Literal / Class / DOT
    fn parse_primary(&mut self) -> Result<AST, Error> {
        self.choice(vec![
            |p| {
                let id = p.parse_identifier()?;
                p.not(|p| {
                    p.expect('<')?;
                    p.expect('-')?;
                    p.parse_spacing()
                })?;
                p.not(|p| {
                    p.expect('-')?;
                    p.expect('>')?;
                    p.parse_spacing()
                })?;
                p.not(|p| {
                    p.parse_identifier()?;
                    p.expect('=')?;
                    p.parse_spacing()
                })?;
                Ok(AST::Identifier(id))
            },
            |p| {
                p.expect('(')?;
                p.parse_spacing()?;
                let expr = p.parse_expression()?;
                p.expect(')')?;
                p.parse_spacing()?;
                Ok(expr)
            },
            |p| p.parse_node(),
            |p| p.parse_list(),
            |p| p.parse_literal(),
            |p| Ok(AST::Choice(p.parse_class()?)),
            |p| {
                p.parse_dot()?;
                Ok(AST::Any)
            },
        ])
    }

    // GR: Node <- OPENC (!CLOSEC Expression)* CLOSEC
    fn parse_node(&mut self) -> Result<AST, Error> {
        self.expect('{')?;
        self.parse_spacing()?;
        let name = self.parse_identifier()?;
        self.expect(':')?;
        self.parse_spacing()?;
        let items = self.zero_or_more(|p| {
            p.not(|p| p.expect('}'))?;
            p.parse_expression()
        })?;
        self.expect('}')?;
        self.parse_spacing()?;
        Ok(AST::Node(name, items))
    }

    // GR: List <- OPENC (!CLOSEC Expression)* CLOSEC
    fn parse_list(&mut self) -> Result<AST, Error> {
        self.expect('{')?;
        self.parse_spacing()?;
        let exprs = self.zero_or_more(|p| {
            p.not(|p| p.expect('}'))?;
            p.parse_expression()
        })?;
        self.expect('}')?;
        self.parse_spacing()?;
        Ok(AST::List(exprs))
    }

    // GR: Identifier <- IdentStart IdentCont* Spacing
    // GR: IdentStart <- [a-zA-Z_]
    // GR: IdentCont <- IdentStart / [0-9]
    fn parse_identifier(&mut self) -> Result<String, Error> {
        let ident_start = self.choice(vec![
            |p| p.expect_range('a', 'z'),
            |p| p.expect_range('A', 'Z'),
            |p| p.expect('_'),
        ])?;
        let ident_cont = self.zero_or_more(|p| {
            p.choice(vec![
                |p| p.expect_range('a', 'z'),
                |p| p.expect_range('A', 'Z'),
                |p| p.expect_range('0', '9'),
                |p| p.expect('_'),
            ])
        })?;
        self.parse_spacing()?;
        let cont_str: String = ident_cont.into_iter().collect();
        let id = format!("{}{}", ident_start, cont_str);
        Ok(id)
    }

    // GR: Literal <- [’] (![’]Char)* [’] Spacing
    // GR:          / ["] (!["]Char)* ["] Spacing
    fn parse_literal(&mut self) -> Result<AST, Error> {
        let litstr = self.parse_literal_string()?;
        Ok(if litstr.len() == 1 {
            AST::Char(litstr.chars().next().unwrap())
        } else {
            AST::String(litstr)
        })
    }

    fn parse_literal_string(&mut self) -> Result<String, Error> {
        self.choice(vec![|p| p.parse_simple_quote(), |p| p.parse_double_quote()])
    }

    fn parse_simple_quote(&mut self) -> Result<String, Error> {
        self.expect('\'')?;
        let r = self
            .zero_or_more(|p| {
                p.not(|p| p.expect('\''))?;
                p.parse_char()
            })?
            .into_iter()
            .collect();
        self.expect('\'')?;
        self.parse_spacing()?;
        Ok(r)
    }

    // TODO: duplicated the above code as I can't pass the quote as a
    // parameter to a more generic function. The `zero_or_more` parser
    // and all the other parsers expect a function pointer, not a
    // closure, and ~const Q: &'static str~ isn't allowed by default.
    fn parse_double_quote(&mut self) -> Result<String, Error> {
        self.expect('"')?;
        let r = self
            .zero_or_more(|p| {
                p.not(|p| p.expect('"'))?;
                p.parse_char()
            })?
            .into_iter()
            .collect();
        self.expect('"')?;
        self.parse_spacing()?;
        Ok(r)
    }

    // GR: Class <- ’[’ (!’]’Range)* ’]’ Spacing
    fn parse_class(&mut self) -> Result<Vec<AST>, Error> {
        self.expect('[')?;
        let output = self.zero_or_more::<AST>(|p| {
            p.not(|pp| pp.expect(']'))?;
            p.parse_range()
        });
        self.expect(']')?;
        self.parse_spacing()?;
        output
    }

    // GR: Range <- Char ’-’ Char / Char
    fn parse_range(&mut self) -> Result<AST, Error> {
        self.choice(vec![
            |p| {
                let left = p.parse_char()?;
                p.expect('-')?;
                Ok(AST::Range(left, p.parse_char()?))
            },
            |p| Ok(AST::Char(p.parse_char()?)),
        ])
    }

    // GR: Char <- ’\\’ [nrt’"\[\]\\]
    // GR:       / ’\\’ [0-2][0-7][0-7]
    // GR:       / ’\\’ [0-7][0-7]?
    // GR:       / !’\\’ .
    fn parse_char(&mut self) -> Result<char, Error> {
        self.choice(vec![|p| p.parse_char_escaped(), |p| {
            p.parse_char_non_escaped()
        }])
    }

    // ’\\’ [nrt’"\[\]\\]
    fn parse_char_escaped(&mut self) -> Result<char, Error> {
        self.expect('\\')?;
        self.choice(vec![
            |p| {
                p.expect('n')?;
                Ok('\n')
            },
            |p| {
                p.expect('r')?;
                Ok('\r')
            },
            |p| {
                p.expect('t')?;
                Ok('\t')
            },
            |p| {
                p.expect('\'')?;
                Ok('\'')
            },
            |p| {
                p.expect('"')?;
                Ok('\"')
            },
            |p| {
                p.expect(']')?;
                Ok(']')
            },
            |p| {
                p.expect('[')?;
                Ok('[')
            },
            |p| {
                p.expect('\\')?;
                Ok('\\')
            },
            |p| {
                p.expect('\'')?;
                Ok('\'')
            },
            |p| {
                p.expect('"')?;
                Ok('"')
            },
        ])
    }

    // !’\\’ .
    fn parse_char_non_escaped(&mut self) -> Result<char, Error> {
        self.not(|p| p.expect('\\'))?;
        self.any()
    }

    // GR: DOT <- '.' Spacing
    fn parse_dot(&mut self) -> Result<char, Error> {
        let r = self.expect('.')?;
        self.parse_spacing()?;
        Ok(r)
    }

    // GR: Spacing <- (Space/ Comment)*
    fn parse_spacing(&mut self) -> Result<(), Error> {
        self.zero_or_more(|p| p.choice(vec![|p| p.parse_space(), |p| p.parse_comment()]))?;
        Ok(())
    }

    // GR: Comment <- ’#’ (!EndOfLine.)* EndOfLine
    fn parse_comment(&mut self) -> Result<(), Error> {
        self.expect('#')?;
        self.zero_or_more(|p| {
            p.not(|p| p.parse_eol())?;
            p.any()
        })?;
        self.parse_eol()
    }

    // GR: Space <- ’ ’ / ’\t’ / EndOfLine
    fn parse_space(&mut self) -> Result<(), Error> {
        self.choice(vec![
            |p| {
                p.expect(' ')?;
                Ok(())
            },
            |p| {
                p.expect('\t')?;
                Ok(())
            },
            |p| p.parse_eol(),
        ])
    }

    // EndOfLine <- ’\r\n’ / ’\n’ / ’\r’
    fn parse_eol(&mut self) -> Result<(), Error> {
        self.choice(vec![
            |p| {
                p.expect('\r')?;
                p.expect('\n')
            },
            |p| p.expect('\n'),
            |p| p.expect('\r'),
        ])?;
        Ok(())
    }

    // EndOfFile <- !.
    fn parse_eof(&mut self) -> Result<(), Error> {
        self.not(|p| p.current())?;
        Ok(())
    }

    fn choice<T>(&mut self, funcs: Vec<ParseFn<T>>) -> Result<T, Error> {
        let cursor = self.cursor;
        for func in &funcs {
            match func(self) {
                Ok(o) => return Ok(o),
                Err(_) => self.cursor = cursor,
            }
        }
        Err(self.err("CHOICE".to_string()))
    }

    fn not<T>(&mut self, func: ParseFn<T>) -> Result<(), Error> {
        let cursor = self.cursor;
        let out = func(self);
        self.cursor = cursor;
        match out {
            Err(_) => Ok(()),
            Ok(_) => Err(self.err("NOT".to_string())),
        }
    }

    fn one_or_more<T>(&mut self, func: ParseFn<T>) -> Result<Vec<T>, Error> {
        let mut output = vec![func(self)?];
        output.append(&mut self.zero_or_more::<T>(func)?);
        Ok(output)
    }

    fn zero_or_more<T>(&mut self, func: ParseFn<T>) -> Result<Vec<T>, Error> {
        let mut output = vec![];
        loop {
            match func(self) {
                Ok(ch) => output.push(ch),
                // stop the loop upon backtrack errors
                Err(Error::BacktrackError(..)) => break,
                // bubble up any error that isn't about input parsing
                Err(e) => return Err(e),
            }
        }
        Ok(output)
    }

    /// If the character under the cursor isn't between `a` and `b`,
    /// return an error, otherwise return the current char and move
    /// the read cursor forward
    fn expect_range(&mut self, a: char, b: char) -> Result<char, Error> {
        let current = self.current()?;
        if current >= a && current <= b {
            self.next();
            return Ok(current);
        }
        Err(self.err(format!(
            "Expected char between `{}' and `{}' but got `{}' instead",
            a, b, current
        )))
    }

    /// Tries to match each character within `expected` against the
    /// input source.  It starts from where the read cursor currently is.
    fn expect_str(&mut self, expected: &str) -> Result<String, Error> {
        for c in expected.chars() {
            self.expect(c)?;
        }
        Ok(expected.to_string())
    }

    /// Compares `expected` to the current character under the cursor,
    /// and advance the cursor if they match.  Returns an error
    /// otherwise.
    fn expect(&mut self, expected: char) -> Result<char, Error> {
        let current = self.current()?;
        if current == expected {
            self.next();
            return Ok(current);
        }
        Err(self.err(format!(
            "Expected `{}' but got `{}' instead",
            expected, current
        )))
    }

    /// If it's not the end of the input, return the current char and
    /// increment the read cursor
    fn any(&mut self) -> Result<char, Error> {
        let current = self.current()?;
        self.next();
        Ok(current)
    }

    /// Retrieve the character within source under the read cursor, or
    /// EOF it it's the end of the input source stream
    fn current(&mut self) -> Result<char, Error> {
        if !self.eof() {
            return Ok(self.source[self.cursor]);
        }
        Err(self.err("EOF".to_string()))
    }

    /// Returns true if the cursor equals the length of the input source
    fn eof(&self) -> bool {
        self.cursor == self.source.len()
    }

    /// Increments the read cursor, and the farther failure position
    /// if it's farther than before the last call
    fn next(&mut self) {
        self.cursor += 1;

        if self.cursor > self.ffp {
            self.ffp = self.cursor;
        }
    }

    /// produce a backtracking error with `message` attached to it
    fn err(&mut self, msg: String) -> Error {
        Error::BacktrackError(self.ffp, msg)
    }
}

pub fn expand(ast: AST) -> Result<AST, Error> {
    Ok(match ast {
        AST::Grammar(definitions) => {
            let defs = definitions
                .into_iter()
                .filter_map(|a| expand(a).ok())
                .collect();
            AST::Grammar(defs)
        }
        AST::Definition(name, expr) => {
            AST::Definition(name.clone(), Box::new(AST::Node(name, vec![*expr])))
        }
        n => n,
    })
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_parse_decimal_number_0() {
        let mut p = Parser::new("0");
        let o = p.parse_number();
        assert!(o.is_ok());
        let u = o.unwrap();
        assert_eq!(SemValue::Number(0), u);
        assert_eq!("0".to_string(), u.to_string());
    }

    #[test]
    fn test_parse_decimal_number_1() {
        let mut p = Parser::new("1");
        let o = p.parse_number();
        assert!(o.is_ok());
        let u = o.unwrap();
        assert_eq!(SemValue::Number(1), u);
        assert_eq!("1".to_string(), u.to_string());
    }

    #[test]
    fn test_parse_decimal_number_2() {
        let mut p = Parser::new("321");
        let o = p.parse_number();
        assert!(o.is_ok());
        let u = o.unwrap();
        assert_eq!(SemValue::Number(321), u);
        assert_eq!("321".to_string(), u.to_string());
    }

    #[test]
    fn test_parse_hexadecimal_number() {
        let mut p = Parser::new("0xfada");
        let o = p.parse_number();
        assert!(o.is_ok());
        let u = o.unwrap();
        assert_eq!(SemValue::Number(0xfada), u);
        assert_eq!("64218".to_string(), u.to_string());
    }

    #[test]
    fn test_parse_sem_expr_decimal_num() {
        let mut p = Parser::new("A -> 0");
        let ast = p.parse_grammar();
        assert!(ast.is_ok());
        assert_eq!("A -> 0\n".to_string(), ast.unwrap().to_string());
    }

    #[test]
    fn test_parse_sem_expr_hex_num() {
        let mut p = Parser::new("A -> 0xff");
        let ast = p.parse_grammar();
        assert!(ast.is_ok());
        assert_eq!("A -> 255\n".to_string(), ast.unwrap().to_string());
    }

    #[test]
    fn test_parse_sem_expr_literal_0() {
        let mut p = Parser::new("A -> 'c'");
        let ast = p.parse_grammar();
        assert!(ast.is_ok());
        assert_eq!("A -> 'c'\n".to_string(), ast.unwrap().to_string());
    }

    #[test]
    fn test_parse_sem_expr_literal_1() {
        let mut p = Parser::new("A -> 'foo'");
        let ast = p.parse_grammar();
        assert!(ast.is_ok());
        assert_eq!("A -> \"foo\"\n".to_string(), ast.unwrap().to_string());
    }

    #[test]
    fn test_parse_sem_expr_variable() {
        let mut p = Parser::new("A -> %0");
        let ast = p.parse_grammar();
        assert!(ast.is_ok());
        assert_eq!("A -> %0\n".to_string(), ast.unwrap().to_string());
    }

    #[test]
    fn test_parse_sem_expr_bool_true() {
        let mut p = Parser::new("true");
        let o = p.parse_bool();
        assert!(o.is_ok());
        let v = o.unwrap();
        assert_eq!("true".to_string(), v.to_string());
        assert_eq!(SemValue::Bool(true), v)
    }

    #[test]
    fn test_parse_sem_expr_bool_false() {
        let mut p = Parser::new("false");
        let o = p.parse_bool();
        assert!(o.is_ok());
        let v = o.unwrap();
        assert_eq!("false".to_string(), v.to_string());
        assert_eq!(SemValue::Bool(false), v)
    }

    #[test]
    fn test_parse_sem_expr_call() {
        let mut p = Parser::new("A -> discard()");
        let ast = p.parse_grammar();
        assert!(ast.is_ok());
        assert_eq!("A -> discard()\n".to_string(), ast.unwrap().to_string());
    }

    #[test]
    fn test_parse_sem_expr_call_one_param() {
        let mut p = Parser::new("A -> cons(%0)");
        let ast = p.parse_grammar();
        assert!(ast.is_ok());
        assert_eq!("A -> cons(%0)\n".to_string(), ast.unwrap().to_string());
    }

    #[test]
    fn test_parse_sem_expr_call_multiparam() {
        let mut p = Parser::new("A -> cons(%0, %1)");
        let ast = p.parse_grammar();
        assert!(ast.is_ok());
        assert_eq!("A -> cons(%0, %1)\n".to_string(), ast.unwrap().to_string());
    }

    #[test]
    fn test_precedence_syntax() {
        let mut p = Parser::new(
            "
            A <- A¹ '+' A² / 'n'
            ",
        );
        let ast = p.parse_grammar();
        assert!(ast.is_ok());
        assert_eq!(
            AST::Grammar(vec![AST::Definition(
                "A".to_string(),
                Box::new(AST::Choice(vec![
                    AST::Sequence(vec![
                        AST::Precedence(Box::new(AST::Identifier("A".to_string())), 1),
                        AST::Char('+'),
                        AST::Precedence(Box::new(AST::Identifier("A".to_string())), 2),
                    ]),
                    AST::Sequence(vec![AST::Char('n'),])
                ])),
            ),]),
            ast.unwrap(),
        );
    }

    #[test]
    fn structure_empty() {
        let mut p = Parser::new(
            "A <- 'a' /
             B <- 'b'
            ",
        );
        let out = p.parse_grammar();

        assert!(out.is_ok());
        assert_eq!(
            AST::Grammar(vec![
                AST::Definition(
                    "A".to_string(),
                    Box::new(AST::Choice(vec![
                        AST::Sequence(vec![AST::Char('a')]),
                        AST::Sequence(vec![AST::Empty])
                    ]))
                ),
                AST::Definition(
                    "B".to_string(),
                    Box::new(AST::Sequence(vec![AST::Char('b')])),
                ),
            ]),
            out.unwrap()
        );
    }

    #[test]
    fn choice_pick_none() -> Result<(), Error> {
        let mut parser = Parser::new("e");
        let out = parser.choice(vec![
            |p| p.expect('a'),
            |p| p.expect('b'),
            |p| p.expect('c'),
            |p| p.expect('d'),
        ]);

        assert!(out.is_err());
        assert_eq!(0, parser.cursor);

        Ok(())
    }

    #[test]
    fn choice_pick_last() -> Result<(), Error> {
        let mut parser = Parser::new("d");
        let out = parser.choice(vec![
            |p| p.expect('a'),
            |p| p.expect('b'),
            |p| p.expect('c'),
            |p| p.expect('d'),
        ]);

        assert!(out.is_ok());
        assert_eq!(1, parser.cursor);

        Ok(())
    }

    #[test]
    fn choice_pick_first() -> Result<(), Error> {
        let mut parser = Parser::new("a");
        let out = parser.choice(vec![|p| p.expect('a')]);

        assert!(out.is_ok());
        assert_eq!(1, parser.cursor);

        Ok(())
    }

    #[test]
    fn not_success_on_err() -> Result<(), Error> {
        let mut parser = Parser::new("a");
        let out = parser.not(|p| p.expect('b'));

        assert!(out.is_ok());
        assert_eq!(0, parser.cursor);

        Ok(())
    }

    #[test]
    fn not_err_on_match() -> Result<(), Error> {
        let mut parser = Parser::new("a");
        let out = parser.not(|p| p.expect('a'));

        assert!(out.is_err());
        assert_eq!(0, parser.cursor);

        Ok(())
    }

    #[test]
    fn zero_or_more() -> Result<(), Error> {
        let mut parser = Parser::new("ab2");

        let prefix = parser.zero_or_more::<char>(|p| p.expect_range('a', 'z'))?;

        assert_eq!(vec!['a', 'b'], prefix);
        assert_eq!(2, parser.cursor);

        Ok(())
    }
}
