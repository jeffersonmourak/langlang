// vm.rs --- parsing machine
//
// This machine is capable of matching patterns in strings.  The
// patterns themselves are expressed in high level text based
// languages and get compiled to programs that can be executed by this
// machine.  This module has nothing to do with how patterns get
// compiled to programs, but how programs get executted as patterns.
//

use std::collections::HashMap;

#[derive(Debug, PartialEq)]
enum Value {
    Chr(char),
    // I64(i64),
    // U64(u64),
    // F64(f64),
    // Str(String),
    Node { name: String, children: Vec<Value> },
}

#[derive(Clone, Debug)]
enum Instruction {
    Halt,
    Any,
    Capture,
    Char(char),
    // Span(char, char),
    Choice(usize),
    Commit(usize),
    CommitB(usize),
    Fail,
    // FailTwice,
    // PartialCommit,
    // BackCommit,
    // TestChar,
    // TestAny,
    Jump(usize),
    Call(usize, usize),
    CallB(usize, usize),
    Return,
    // Throw(usize),
}

#[derive(Clone, Debug, PartialEq)]
enum Error {
    Fail,
    LeftRec,
    Overflow,
    Matching(String),
    EOF,
}

#[derive(Debug)]
struct Program {
    names: HashMap<usize, String>,
    code: Vec<Instruction>,
    // source_mapping: ...
}

#[derive(Debug, PartialEq)]
enum StackFrameType {
    Backtrack,
    Call,
}

#[derive(Debug)]
struct StackFrame {
    ftype: StackFrameType,
    program_counter: usize,       // pc
    cursor: Result<usize, Error>, // s
    result: Result<usize, Error>, // X
    address: usize,               // pc+l
    precedence: usize,            // k
    capture: usize,
    captures: Vec<Value>,
}

impl StackFrame {
    fn new_backtrack(cursor: usize, pc: usize, capture: usize) -> Self {
        StackFrame {
            ftype: StackFrameType::Backtrack,
            program_counter: pc,
            capture: capture,
            cursor: Ok(cursor),
            // fields not used for backtrack frames
            captures: vec![],
            address: 0,
            result: Err(Error::Fail),
            precedence: 0,
        }
    }

    fn new_call(pc: usize, address: usize, precedence: usize) -> Self {
        StackFrame {
            ftype: StackFrameType::Call,
            program_counter: pc,
            cursor: Err(Error::Fail),
            result: Err(Error::Fail),
            address: address,
            precedence: precedence,
            capture: 0,
            captures: vec![],
        }
    }

    fn new_lrcall(cursor: usize, pc: usize, address: usize, precedence: usize) -> Self {
        StackFrame {
            ftype: StackFrameType::Call,
            program_counter: pc,
            cursor: Ok(cursor),
            result: Err(Error::LeftRec),
            address: address,
            precedence: precedence,
            captures: vec![],
            capture: 0,
        }
    }
}

// #[derive(Debug)]
// enum Status {
//     Halt,
//     Continue,
// }

// pc+l: production address
//    s: subject, cursor index
type LeftRecTableKey = (usize, usize);

//         s': subject in left recursive call
// precedence: precedence level in recursive call
#[derive(Debug)]
struct LeftRecTableEntry {
    cursor: Result<usize, Error>,
    precedence: usize,
    bound: usize,
}

#[derive(Debug)]
struct VM {
    // Cursor within input, error means matching failed
    cursor: Result<usize, Error>,
    // Farther Failure Position
    ffp: usize,
    // Input source
    source: Vec<char>,
    // Vector of instructions and tables with literal values
    program: Program,
    // Cursor within the program
    program_counter: usize,
    // Stack of both backtrack and call frames
    stack: Vec<StackFrame>,
    // Memoized position of left recursive results
    lrmemo: HashMap<LeftRecTableKey, LeftRecTableEntry>,
    // Value returned from last operation
    accumulator: Option<Value>,
}

impl VM {
    fn new(program: Program) -> Self {
        VM {
            ffp: 0,
            cursor: Ok(0),
            source: vec![],
            program: program,
            program_counter: 0,
            stack: vec![],
            lrmemo: HashMap::new(),
            accumulator: None,
        }
    }

    fn advance_cursor(&mut self) -> Result<(), Error> {
        let cursor = self.cursor.clone()? + 1;
        if cursor > self.ffp {
            self.ffp = cursor;
        }
        self.cursor = Ok(cursor);
        Ok(())
    }

    // stack management

    fn stkpeek(&mut self) -> Result<&mut StackFrame, Error> {
        let len = self.stack.len();
        if len < 1 {
            return Err(Error::Overflow)?;
        }
        Ok(&mut self.stack[len - 1])
    }

    fn stkpop(&mut self) -> Result<StackFrame, Error> {
        self.stack.pop().ok_or(Error::Overflow)
    }

    // functions for capturing matched values

    fn capture(&mut self, v: Value) -> Result<(), Error> {
        for i in (0..self.stack.len()).rev() {
            if self.stack[i].ftype == StackFrameType::Backtrack {
                continue;
            }
            self.stack[i].captures.push(v);
            break;
        }
        Ok(())
    }

    fn pop_captured(&mut self, how_many: usize) {
        if let Ok(frame) = self.stkpeek() {
            for _ in 0..(frame.captures.len() - how_many) {
                frame.captures.pop();
            }
        }
    }

    fn captured_len(&self) -> Result<usize, Error> {
        let len = self.stack.len();
        if len > 0 {
            return Ok(self.stack[len - 1].captures.len());
        }
        Err(Error::Overflow)
    }

    // evaluation

    fn run(&mut self, input: &String) -> Result<(), Error> {
        self.source = input.chars().collect();

        loop {
            if self.program_counter >= self.program.code.len() {
                return Err(Error::Overflow);
            }

            let (instruction, cursor) = match self.cursor {
                Ok(c) => (self.program.code[self.program_counter].clone(), c),
                Err(_) => (Instruction::Fail, 0),
            };

            println!("[{:?}] I: {:?}", cursor, instruction);

            match instruction {
                Instruction::Halt => break,
                Instruction::Any => {
                    if cursor >= self.source.len() {
                        self.cursor = Err(Error::EOF);
                    } else {
                        self.accumulator = Some(Value::Chr(self.source[cursor]));
                        self.advance_cursor()?;
                        self.program_counter += 1;
                    }
                }
                Instruction::Char(expected) => {
                    if cursor >= self.source.len() {
                        self.cursor = Err(Error::EOF);
                        continue;
                    }
                    let current = self.source[cursor];
                    if current != expected {
                        self.cursor = Err(Error::Matching(format!(
                            "Expected {}, but got {} instead",
                            expected, current,
                        )));
                        continue;
                    }
                    self.accumulator = Some(Value::Chr(self.source[cursor]));
                    self.advance_cursor()?;
                    self.program_counter += 1;
                }
                Instruction::Choice(offset) => {
                    self.stack.push(StackFrame::new_backtrack(
                        cursor,
                        self.program_counter + offset,
                        self.captured_len()?,
                    ));
                    self.program_counter += 1;
                }
                Instruction::Commit(offset) => {
                    self.stack.pop();
                    self.program_counter += offset;
                }
                Instruction::CommitB(offset) => {
                    self.stack.pop();
                    self.program_counter -= offset;
                }
                Instruction::Fail => {
                    self.fail()?;
                }
                Instruction::Jump(index) => {
                    self.program_counter = index;
                }
                Instruction::Call(offset, precedence) => {
                    self.inst_call(self.program_counter + offset, precedence)?;
                }
                Instruction::CallB(offset, precedence) => {
                    self.inst_call(self.program_counter - offset, precedence)?;
                }
                Instruction::Return => {
                    self.inst_return()?;
                }
                Instruction::Capture => {
                    if let Some(v) = self.accumulator.take() {
                        self.capture(v)?;
                    }
                    self.program_counter += 1;
                }
            }
        }
        Ok(())
    }

    fn inst_call(&mut self, address: usize, precedence: usize) -> Result<(), Error> {
        let cursor = self.cursor.clone()?;
        if precedence == 0 {
            self.stack.push(StackFrame::new_call(
                self.program_counter + 1,
                address,
                precedence,
            ));
            self.program_counter = address;
            return Ok(());
        }
        let key = (address, cursor);
        match self.lrmemo.get(&key) {
            None => {
                // println!("lvar.{{1, 2}}");
                self.stack.push(StackFrame::new_lrcall(
                    cursor,
                    self.program_counter + 1,
                    address,
                    precedence,
                ));
                self.program_counter = address;
                self.lrmemo.insert(
                    key,
                    LeftRecTableEntry {
                        cursor: Err(Error::LeftRec),
                        precedence: precedence,
                        bound: 0,
                    },
                );
            }
            Some(entry) => {
                match entry.cursor {
                    Err(_) => {
                        // println!("lvar.{{3, 5}}.1");
                        self.fail()?;
                    }
                    Ok(cursor) => {
                        if precedence < entry.precedence {
                            // println!("lvar.{{3, 5}}.2");
                            self.fail()?;
                        } else {
                            // println!("lvar.4");
                            self.cursor = Ok(cursor);
                            self.program_counter += 1;
                        }
                    }
                }
            }
        }
        Ok(())
    }

    fn inst_return(&mut self) -> Result<(), Error> {
        let cursor = self.cursor.clone()?;
        let mut frame = self.stkpeek()?;
        let address = frame.address;

        if frame.precedence == 0 {
            let frame = self.stkpop()?;
            self.program_counter = frame.program_counter;
            let name = self
                .program
                .names
                .get(&address)
                .unwrap_or(&"".to_string())
                .clone();
            self.accumulator = Some(Value::Node {
                name: name,
                children: frame.captures,
            });
            return Ok(());
        }

        if frame.result.is_err() || cursor > frame.result.clone()? {
            // println!("{{lvar, inc}}.1");

            frame.result = Ok(cursor);

            let frame_cursor = frame.cursor.clone();
            let key = (frame.address, frame_cursor.clone().unwrap());
            let mut entry = &mut self.lrmemo.get_mut(&key).ok_or(Error::Fail)?;

            entry.cursor = Ok(cursor);
            entry.bound += 1;
            entry.precedence = entry.precedence;

            self.cursor = frame_cursor;
            self.program_counter = address;
        } else {
            // println!("inc.3");
            let pc = frame.program_counter;
            self.cursor = frame.result.clone();
            self.program_counter = pc;
            self.stkpop()?;
        }
        Ok(())
    }

    fn fail(&mut self) -> Result<(), Error> {
        // println!("::::::FAIL");
        let error = match self.cursor.clone() {
            Err(e) => e,
            Ok(_) => Error::Fail,
        };
        let frame = loop {
            match self.stack.pop() {
                None => {
                    self.cursor = Err(error.clone());
                    return Err(error);
                }
                Some(f) => {
                    if let Ok(cursor) = f.cursor {
                        self.cursor = Ok(cursor);
                        break f;
                    }
                }
            }
        };

        self.program_counter = frame.program_counter;
        self.pop_captured(frame.capture);
        Ok(())
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    // (ch.1)
    //
    // s[i] = 'c'
    // -------------------
    // match 'c' s i = i+1
    #[test]
    fn ch_1() {
        let input = "a".to_string();
        // G <- 'a'
        let program = Program {
            names: HashMap::new(),
            code: vec![
                Instruction::Call(2, 0),
                Instruction::Halt,
                Instruction::Char('a'),
                Instruction::Return,
            ],
        };

        let mut vm = VM::new(program);
        let result = vm.run(&input);

        assert!(result.is_ok());
        assert!(vm.cursor.is_ok());
        assert_eq!(1, vm.cursor.unwrap());
    }

    // (ch.2)
    //
    // s[i] != 'c'
    // -------------------
    // match 'c' s i = nil
    #[test]
    fn ch_2() {
        let input = "b".to_string();
        // G <- 'a'
        let program = Program {
            names: HashMap::new(),
            code: vec![
                Instruction::Call(2, 0),
                Instruction::Halt,
                Instruction::Char('a'),
                Instruction::Return,
            ],
        };

        let mut vm = VM::new(program);
        let result = vm.run(&input);

        assert!(result.is_err());
        assert_eq!(
            Error::Matching("Expected a, but got b instead".to_string()),
            result.unwrap_err(),
        );
        assert_eq!(0, vm.ffp);
    }

    // (any.1)
    //   i ≤ |s|
    // -----------------
    // match . s i = i+1
    #[test]
    fn any_1() {
        let input = "abcd".to_string();
        let program = Program {
            names: HashMap::new(),
            code: vec![
                Instruction::Call(2, 0),
                Instruction::Halt,
                Instruction::Any,
                Instruction::Any,
                Instruction::Any,
                Instruction::Return,
            ],
        };

        let mut vm = VM::new(program);
        let result = vm.run(&input);

        assert!(result.is_ok());
        assert!(vm.cursor.is_ok());
        assert_eq!(3, vm.cursor.unwrap());
    }

    // (any.2)
    // i > |s|
    // -----------------
    // match . s i = nil
    #[test]
    fn any_2_eof() {
        let input = "".to_string();
        let program = Program {
            names: HashMap::new(),
            code: vec![
                Instruction::Call(2, 0),
                Instruction::Halt,
                Instruction::Any,
                Instruction::Return,
            ],
        };

        let mut vm = VM::new(program);
        let result = vm.run(&input);

        assert!(result.is_err());
        assert_eq!(Error::EOF, result.clone().unwrap_err());
        assert!(vm.cursor.is_err());
        assert_eq!(vm.cursor.unwrap_err(), result.unwrap_err())
    }

    // (not.1)
    // match p s i = nil
    // -----------------
    // match !p s i = i
    #[test]
    fn not_1() {
        let input = "foo".to_string();
        // G <- !'a'
        let program = Program {
            names: HashMap::new(),
            code: vec![
                Instruction::Call(2, 0),
                Instruction::Halt,
                Instruction::Choice(4),
                Instruction::Char('a'),
                Instruction::Commit(1),
                Instruction::Fail,
                Instruction::Return,
            ],
        };

        let mut vm = VM::new(program);
        let result = vm.run(&input);

        assert!(result.is_ok());
        assert!(vm.cursor.is_ok());
        assert_eq!(0, vm.cursor.unwrap());
        assert_eq!(0, vm.ffp);
    }

    // (not.2)
    // match p s i = i+j
    // ------------------
    // match !p s i = nil
    #[test]
    fn not_2() {
        let input = "foo".to_string();
        // G <- !'f'
        let program = Program {
            names: HashMap::new(),
            code: vec![
                Instruction::Call(2, 0),
                Instruction::Halt,
                Instruction::Choice(4),
                Instruction::Char('f'),
                Instruction::Commit(1),
                Instruction::Fail,
                Instruction::Return,
            ],
        };

        let mut vm = VM::new(program);
        let result = vm.run(&input);

        assert!(result.is_err());
        assert_eq!(Error::Fail, result.unwrap_err());
        assert!(vm.cursor.is_err());
        assert_eq!(1, vm.ffp);
    }

    // (ord.1)
    // match p1 s i = nil    match p2 s i = nil
    // ----------------------------------------
    //         match p1 / p2 s i = nil
    #[test]
    fn ord_1() {
        let input = "c".to_string();
        // G <- 'a' / 'b'
        let program = Program {
            names: HashMap::new(),
            code: vec![
                Instruction::Call(2, 0),
                Instruction::Halt,
                Instruction::Choice(3),
                Instruction::Char('a'),
                Instruction::Commit(2),
                Instruction::Char('b'),
                Instruction::Halt,
                Instruction::Return,
            ],
        };

        let mut vm = VM::new(program);
        let result = vm.run(&input);

        assert!(result.is_err());
        // currently shows the last error
        assert_eq!(
            Error::Matching("Expected b, but got c instead".to_string()),
            result.unwrap_err()
        );
        assert_eq!(0, vm.ffp);
    }

    // (ord.2)
    // match p1 s i = i+j
    // -----------------------
    // match p1 / p2 s i = i+j (ord.2)
    #[test]
    fn ord_2() {
        let input = "a".to_string();
        // G <- 'a' / 'b'
        let program = Program {
            names: HashMap::new(),
            code: vec![
                Instruction::Call(2, 0),
                Instruction::Halt,
                Instruction::Choice(3),
                Instruction::Char('a'),
                Instruction::Commit(2),
                Instruction::Char('b'),
                Instruction::Halt,
                Instruction::Return,
            ],
        };

        let mut vm = VM::new(program);
        let result = vm.run(&input);

        assert!(result.is_ok());
        assert!(vm.cursor.is_ok());
        assert_eq!(1, vm.cursor.unwrap());
    }

    // (ord.3)
    // match p1 s i = nil    match p2 s i = i+k
    // ----------------------------------------
    //         match p1 / p2 s i = i+k
    #[test]
    fn ord_3() {
        let input = "b".to_string();
        // G <- 'a' / 'b'
        let program = Program {
            names: HashMap::new(),
            code: vec![
                Instruction::Call(2, 0),
                Instruction::Halt,
                Instruction::Choice(3),
                Instruction::Char('a'),
                Instruction::Commit(2),
                Instruction::Char('b'),
                Instruction::Halt,
                Instruction::Return,
            ],
        };

        let mut vm = VM::new(program);
        let result = vm.run(&input);

        assert!(result.is_ok());
        assert!(vm.cursor.is_ok());
        assert_eq!(1, vm.cursor.unwrap());
        assert_eq!(1, vm.ffp);
    }

    // (rep.1)
    // match p s i = i+j    match p∗ s i + j = i+j+k
    // ----------------------------------------------
    //            match p∗ s i = i+j+k
    #[test]
    fn rep_1() {
        let input = "aab".to_string();
        // G <- 'a*'
        let program = Program {
            names: HashMap::new(),
            code: vec![
                Instruction::Call(2, 0),
                Instruction::Halt,
                Instruction::Choice(3),
                Instruction::Char('a'),
                Instruction::CommitB(2),
                Instruction::Return,
            ],
        };

        let mut vm = VM::new(program);
        let result = vm.run(&input);

        assert!(result.is_ok());
        assert!(vm.cursor.is_ok());
        assert_eq!(2, vm.cursor.unwrap());
        assert_eq!(2, vm.ffp);
    }

    // (rep.2)
    // match p s i = nil
    // -----------------
    // match p∗ s i = i
    #[test]
    fn rep_2() {
        let input = "b".to_string();
        // G <- 'a*'
        let program = Program {
            names: HashMap::new(),
            code: vec![
                Instruction::Call(2, 0),
                Instruction::Halt,
                Instruction::Choice(3),
                Instruction::Char('a'),
                Instruction::CommitB(2),
                Instruction::Return,
            ],
        };

        let mut vm = VM::new(program);
        let result = vm.run(&input);

        assert!(result.is_ok());
        assert!(vm.cursor.is_ok());
        assert_eq!(0, vm.cursor.unwrap());
        assert_eq!(0, vm.ffp);
    }

    // (var.1)
    // match g g(Ak) s i = i+j
    // -----------------------
    // match g Ak s i = i+j
    #[test]
    fn var_1() {
        let input = "1+1".to_string();
        // G <- D '+' D
        // D <- '0' / '1'
        let program = Program {
            names: HashMap::new(),
            code: vec![
                Instruction::Call(2, 0),
                Instruction::Jump(11),
                // G
                Instruction::Call(4, 0),
                Instruction::Char('+'),
                Instruction::Call(2, 0),
                Instruction::Return,
                // D
                Instruction::Choice(3),
                Instruction::Char('0'),
                Instruction::Commit(2),
                Instruction::Char('1'),
                Instruction::Return,
                Instruction::Halt,
            ],
        };

        let mut vm = VM::new(program);
        let result = vm.run(&input);

        assert!(result.is_ok());
        assert!(vm.cursor.is_ok());
        assert_eq!(3, vm.cursor.unwrap());
        assert_eq!(3, vm.ffp);
    }

    // (var.2)
    // match g g(Ak) s i = nil
    // -----------------------
    //  match g Ak s i = nil
    #[test]
    fn var_2() {
        let input = "1+2".to_string();
        // G <- D '+' D
        // D <- '0' / '1'
        let program = Program {
            names: HashMap::new(),
            code: vec![
                Instruction::Call(2, 0),
                Instruction::Jump(11),
                // G
                Instruction::Call(4, 0),
                Instruction::Char('+'),
                Instruction::Call(2, 0),
                Instruction::Return,
                // D
                Instruction::Choice(3),
                Instruction::Char('0'),
                Instruction::Commit(2),
                Instruction::Char('1'),
                Instruction::Return,
                Instruction::Halt,
            ],
        };

        let mut vm = VM::new(program);
        let result = vm.run(&input);

        assert!(result.is_err());
        assert_eq!(
            Error::Matching("Expected 1, but got 2 instead".to_string()),
            result.unwrap_err()
        );
        assert_eq!(2, vm.ffp);
    }

    // (lvar.1)
    #[test]
    fn lrvar_1() {
        let input = "n+n".to_string();
        // G <- G '+' 'n' / 'n'
        let program = Program {
            names: HashMap::new(),
            code: vec![
                Instruction::Call(2, 1),
                Instruction::Jump(9),
                Instruction::Choice(5),
                Instruction::CallB(1, 1),
                Instruction::Char('+'),
                Instruction::Char('n'),
                Instruction::Commit(2),
                Instruction::Char('n'),
                Instruction::Return,
                Instruction::Halt,
            ],
        };

        let mut vm = VM::new(program);
        let result = vm.run(&input);

        assert!(result.is_ok());
        assert!(vm.cursor.is_ok());
        assert_eq!(3, vm.cursor.unwrap());
    }

    #[test]
    fn capture_1() {
        // G <- 'abacate' / 'abada'
        let values = [(2, "G".to_string())].iter().cloned().collect();
        let program = Program {
            names: values,
            code: vec![
                // Call to first production follwed by the end of the matching
                Instruction::Call(2, 0),
                Instruction::Halt,
                // Body of the production G
                Instruction::Choice(16),
                Instruction::Char('a'),
                Instruction::Capture,
                Instruction::Char('b'),
                Instruction::Capture,
                Instruction::Char('a'),
                Instruction::Capture,
                Instruction::Char('c'),
                Instruction::Capture,
                Instruction::Char('a'),
                Instruction::Capture,
                Instruction::Char('t'),
                Instruction::Capture,
                Instruction::Char('e'),
                Instruction::Capture,
                Instruction::Commit(11),
                Instruction::Char('a'),
                Instruction::Capture,
                Instruction::Char('b'),
                Instruction::Capture,
                Instruction::Char('a'),
                Instruction::Capture,
                Instruction::Char('d'),
                Instruction::Capture,
                Instruction::Char('a'),
                Instruction::Capture,
                Instruction::Return,
            ],
        };

        let mut vm = VM::new(program);
        let input = "abada".to_string();
        let result = vm.run(&input);

        assert!(result.is_ok());
        assert!(vm.cursor.is_ok());
        assert_eq!(5, vm.cursor.unwrap());
        assert!(vm.accumulator.is_some());
        assert_eq!(
            Value::Node {
                name: "G".to_string(),
                children: vec![
                    Value::Chr('a'),
                    Value::Chr('b'),
                    Value::Chr('a'),
                    Value::Chr('d'),
                    Value::Chr('a'),
                ]
            },
            vm.accumulator.unwrap()
        );
    }

    #[test]
    fn capture_2() {
        // G <- D
        // D <- '0' / '1'
        let values = [(2, "G".to_string()), (5, "D".to_string())]
            .iter()
            .cloned()
            .collect();
        let program = Program {
            names: values,
            code: vec![
                Instruction::Call(2, 0),
                Instruction::Halt,
                // G
                Instruction::Call(3, 0),
                Instruction::Capture,
                Instruction::Return,
                // D
                Instruction::Choice(4),
                Instruction::Char('0'),
                Instruction::Capture,
                Instruction::Commit(3),
                Instruction::Char('1'),
                Instruction::Capture,
                Instruction::Return,
            ],
        };

        let mut vm = VM::new(program);
        let input = "1".to_string();
        let result = vm.run(&input);

        assert!(result.is_ok());
        assert!(vm.cursor.is_ok());
        assert_eq!(1, vm.cursor.unwrap());
        assert!(vm.accumulator.is_some());
        assert_eq!(
            Value::Node {
                name: "G".to_string(),
                children: vec![Value::Node {
                    name: "D".to_string(),
                    children: vec![Value::Chr('1')],
                }],
            },
            vm.accumulator.unwrap()
        );
    }
}
